// Command officedup finds duplicated and drifted function bodies across the
// apps/backend/internal/office/** package tree.
//
// Detection is AST-based, not text-based: each function body is re-printed with
// go/printer (which drops comments and normalizes all whitespace/formatting),
// with the receiver identifier rewritten to a fixed token so that `s *Service`
// and `m *Manager` copies hash identically. Two functions are "identical" when
// those normalized bodies are byte-equal. Near-duplicates are scored by Jaccard
// similarity over the go/scanner token multiset of the same normalized body.
//
// Usage: go run . <office-dir>
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"go/ast"
	"go/parser"
	"go/printer"
	"go/scanner"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type fn struct {
	Pkg      string // office-relative package dir, e.g. "service" or "agents"
	File     string
	Line     int
	Name     string // Recv.Name for methods, else bare name
	Recv     string
	Lines    int
	Body     string // normalized
	Hash     string
	Tokens   map[string]int
	TokCount int
}

func (f fn) ID() string { return f.Pkg + "." + f.Name }

func main() {
	root := os.Args[1]
	fset := token.NewFileSet()
	var fns []fn

	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(path, ".go") {
			return err
		}
		if strings.HasSuffix(path, "_test.go") {
			return nil
		}
		file, perr := parser.ParseFile(fset, path, nil, 0) // mode 0 => comments dropped
		if perr != nil {
			fmt.Fprintln(os.Stderr, "parse:", perr)
			return nil
		}
		rel, _ := filepath.Rel(root, filepath.Dir(path))
		for _, decl := range file.Decls {
			fd, ok := decl.(*ast.FuncDecl)
			if !ok || fd.Body == nil {
				continue
			}
			recv := ""
			if fd.Recv != nil && len(fd.Recv.List) > 0 && len(fd.Recv.List[0].Names) > 0 {
				recv = fd.Recv.List[0].Names[0].Name
			}
			if recv != "" {
				normalizeRecv(fd.Body, recv)
			}
			var sb strings.Builder
			if perr := (&printer.Config{Mode: printer.RawFormat}).Fprint(&sb, fset, fd.Body); perr != nil {
				continue
			}
			body := strings.Join(strings.Fields(sb.String()), " ")
			start := fset.Position(fd.Pos())
			end := fset.Position(fd.End())
			nLines := end.Line - start.Line + 1
			if nLines < 6 || len(body) < 120 {
				continue // same floor as the original heuristic
			}
			sum := sha256.Sum256([]byte(body))
			toks, n := tokenize(sb.String())
			fns = append(fns, fn{
				Pkg: rel, File: path, Line: start.Line, Name: fd.Name.Name, Recv: recv,
				Lines: nLines, Body: body, Hash: hex.EncodeToString(sum[:8]),
				Tokens: toks, TokCount: n,
			})
		}
		return nil
	})
	if err != nil {
		panic(err)
	}

	fmt.Printf("## parsed %d functions (>=6 lines, >=120 chars normalized)\n\n", len(fns))

	// --- Section A: byte-identical normalized bodies spanning >1 package.
	byHash := map[string][]fn{}
	for _, f := range fns {
		byHash[f.Hash] = append(byHash[f.Hash], f)
	}
	type group struct {
		hash string
		fs   []fn
	}
	var ident []group
	for h, g := range byHash {
		pkgs := map[string]bool{}
		for _, f := range g {
			pkgs[f.Pkg] = true
		}
		if len(pkgs) > 1 {
			ident = append(ident, group{h, g})
		}
	}
	sort.Slice(ident, func(i, j int) bool { return ident[i].fs[0].Name < ident[j].fs[0].Name })
	fmt.Printf("## SECTION A — identical normalized bodies across packages (%d groups)\n\n", len(ident))
	for _, g := range ident {
		sort.Slice(g.fs, func(i, j int) bool { return g.fs[i].Pkg < g.fs[j].Pkg })
		names := map[string]bool{}
		var locs []string
		for _, f := range g.fs {
			names[f.Name] = true
			locs = append(locs, fmt.Sprintf("%s:%d(%s.%s,%dL)", f.File, f.Line, f.Pkg, f.Name, f.Lines))
		}
		tag := ""
		if len(names) > 1 {
			tag = " [RENAMED]"
		}
		fmt.Printf("- %s%s\n    %s\n", g.fs[0].Name, tag, strings.Join(locs, "\n    "))
	}

	// --- Section B: near-duplicates between office/service and every other package.
	var svc, other []fn
	for _, f := range fns {
		if f.Pkg == "service" {
			svc = append(svc, f)
		} else {
			other = append(other, f)
		}
	}
	type pair struct {
		a, b fn
		sim  float64
	}
	var near []pair
	for _, a := range svc {
		for _, b := range other {
			if a.Hash == b.Hash {
				continue // already in Section A
			}
			s := jaccard(a.Tokens, b.Tokens)
			if s < 0.60 {
				continue
			}
			near = append(near, pair{a, b, s})
		}
	}
	sort.Slice(near, func(i, j int) bool { return near[i].sim > near[j].sim })
	fmt.Printf("\n## SECTION B — near-duplicate service<->subpackage pairs, sim>=0.60 (%d pairs)\n\n", len(near))
	for _, p := range near {
		same := ""
		if p.a.Name == p.b.Name {
			same = " SAME-NAME"
		}
		fmt.Printf("%.3f%s  service.%s (%s:%d,%dL)  <->  %s.%s (%s:%d,%dL)\n",
			p.sim, same, p.a.Name, p.a.File, p.a.Line, p.a.Lines,
			p.b.Pkg, p.b.Name, p.b.File, p.b.Line, p.b.Lines)
	}
}

// normalizeRecv rewrites every use of the receiver identifier to a fixed token
// so that copies differing only in receiver name (s/m/svc) hash identically.
func normalizeRecv(body *ast.BlockStmt, recv string) {
	ast.Inspect(body, func(n ast.Node) bool {
		if id, ok := n.(*ast.Ident); ok && id.Name == recv {
			id.Name = "RCV"
		}
		return true
	})
}

func tokenize(src string) (map[string]int, int) {
	var s scanner.Scanner
	fset := token.NewFileSet()
	f := fset.AddFile("", fset.Base(), len(src))
	s.Init(f, []byte(src), nil, 0)
	out := map[string]int{}
	n := 0
	for {
		_, tok, lit := s.Scan()
		if tok == token.EOF {
			break
		}
		k := tok.String()
		if lit != "" {
			k = lit
		}
		out[k]++
		n++
	}
	return out, n
}

// jaccard over token multisets: sum(min)/sum(max).
func jaccard(a, b map[string]int) float64 {
	seen := map[string]bool{}
	var inter, union int
	for k, va := range a {
		seen[k] = true
		vb := b[k]
		inter += min(va, vb)
		union += max(va, vb)
	}
	for k, vb := range b {
		if seen[k] {
			continue
		}
		union += vb
	}
	if union == 0 {
		return 0
	}
	return float64(inter) / float64(union)
}
