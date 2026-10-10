// worktree-inventory-repair previews and applies explicit offline inventory repairs.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/kandev/kandev/internal/task/inventoryrepair"
)

func main() {
	if err := run(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string, out, diagnostics io.Writer) error {
	flags := flag.NewFlagSet("worktree-inventory-repair", flag.ContinueOnError)
	flags.SetOutput(diagnostics)
	path := flags.String("plan", "", "explicit JSON repair plan (required)")
	apply := flags.Bool("apply", false, "apply or resume the reviewed plan with the backend stopped")
	rollback := flags.Bool("rollback", false, "reverse an unchanged repair with the backend stopped")
	verify := flags.Bool("verify", false, "verify a completed repair with the backend stopped")
	if err := flags.Parse(args); err != nil {
		return err
	}
	modes := 0
	for _, enabled := range []bool{*apply, *rollback, *verify} {
		if enabled {
			modes++
		}
	}
	if *path == "" || flags.NArg() != 0 || modes > 1 {
		return errors.New("provide --plan FILE and at most one of --apply, --rollback, --verify; default is read-only preview")
	}
	f, err := os.Open(*path)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	p, err := inventoryrepair.Decode(f)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	encoder := json.NewEncoder(out)
	encoder.SetIndent("", "  ")
	if modes == 0 {
		report, err := inventoryrepair.Preview(ctx, p)
		if err != nil {
			return err
		}
		return encoder.Encode(report)
	}
	mode := "applied"
	switch {
	case *apply:
		err = inventoryrepair.Apply(ctx, p)
	case *rollback:
		mode = "rolled_back"
		err = inventoryrepair.Rollback(ctx, p)
	case *verify:
		mode = "verified"
		err = inventoryrepair.Verify(ctx, p)
	}
	if err != nil {
		return err
	}
	return encoder.Encode(map[string]string{"operation_id": p.OperationID, "status": mode})
}
