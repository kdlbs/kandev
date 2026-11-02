package sandbox

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"path/filepath"
)

func (a *AuditArchive) freezeFile(ctx context.Context, root *os.Root, name string, source RuntimeSource, previous []AuditSourceReceipt) error {
	info, err := root.Lstat(filepath.FromSlash(name))
	if err != nil || !info.Mode().IsRegular() || info.Size() > int64(source.MaxBytes) || a.totalBytes()+info.Size() > maximumAuditArchiveBytes {
		return ErrInvalidPolicy
	}
	original, err := root.Open(filepath.FromSlash(name))
	if err != nil {
		return err
	}
	defer func() { _ = original.Close() }()
	before, err := original.Stat()
	if err != nil || !os.SameFile(info, before) {
		return ErrInvalidPolicy
	}
	identity, err := auditFileIdentity(original)
	if err != nil {
		return err
	}
	idHash := sha256.Sum256([]byte(source.Key + "\x00" + name))
	receipt := AuditSourceReceipt{ID: hex.EncodeToString(idHash[:]), Schema: source.Key, Name: name, Identity: identity, Size: info.Size()}
	if a.duplicateIdentity(receipt) {
		return ErrInvalidPolicy
	}
	if err = verifyPriorAuditBytes(ctx, original, receipt, previous); err != nil {
		return err
	}
	copied, err := os.CreateTemp(a.root, "source-")
	if err != nil {
		return err
	}
	retained := false
	defer func() {
		if !retained {
			_ = copied.Close()
		}
	}()
	receipt.Digest, err = copyFrozenAudit(ctx, original, copied, before, info.Size())
	if err != nil {
		return err
	}
	a.files = append(a.files, copied)
	a.Sources = append(a.Sources, receipt)
	retained = true
	return nil
}

func verifyPriorAuditBytes(ctx context.Context, file *os.File, current AuditSourceReceipt, previous []AuditSourceReceipt) error {
	for _, prior := range previous {
		if prior.ID != current.ID {
			continue
		}
		if prior.Schema != current.Schema || prior.Identity != current.Identity || prior.Size > current.Size {
			return ErrInvalidPolicy
		}
		digest := sha256.New()
		count, err := io.CopyBuffer(digest, &auditContextReader{ctx: ctx, reader: io.LimitReader(file, prior.Size)}, make([]byte, 64<<10))
		if err != nil || count != prior.Size || hex.EncodeToString(digest.Sum(nil)) != prior.Digest {
			return ErrInvalidPolicy
		}
		if _, err = file.Seek(0, io.SeekStart); err != nil {
			return err
		}
		return nil
	}
	return nil
}

func (a *AuditArchive) totalBytes() int64 {
	var total int64
	for _, source := range a.Sources {
		total += source.Size
	}
	return total
}

type auditContextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (r *auditContextReader) Read(buffer []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	count, err := r.reader.Read(buffer)
	if cancelled := r.ctx.Err(); cancelled != nil {
		return count, cancelled
	}
	return count, err
}

func (a *AuditArchive) duplicateIdentity(receipt AuditSourceReceipt) bool {
	for _, source := range a.Sources {
		if source.ID == receipt.ID || source.Identity == receipt.Identity {
			return true
		}
	}
	return false
}

func copyFrozenAudit(ctx context.Context, original, copied *os.File, before os.FileInfo, size int64) (string, error) {
	digest := sha256.New()
	written, err := io.CopyBuffer(io.MultiWriter(copied, digest), &auditContextReader{ctx: ctx, reader: io.LimitReader(original, size+1)}, make([]byte, 64<<10))
	after, statErr := original.Stat()
	if err != nil || statErr != nil || written != size || !os.SameFile(before, after) || after.Size() != before.Size() || !after.ModTime().Equal(before.ModTime()) {
		return "", ErrInvalidPolicy
	}
	return hex.EncodeToString(digest.Sum(nil)), nil
}
