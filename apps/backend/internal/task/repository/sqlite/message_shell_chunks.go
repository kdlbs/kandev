package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"math"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
)

const shellOutputChunkBytes = 4096
const shellOutputStorageKey = "shell_output_storage"

type shellOutputHead struct {
	Identity    string
	Header      string
	StdoutStart int64
	StdoutEnd   int64
	StderrStart int64
	StderrEnd   int64
	Sequence    int64
}

func loadShellOutputHead(ctx context.Context, tx *sqlx.Tx, messageID string) (*shellOutputHead, error) {
	head := &shellOutputHead{}
	err := tx.QueryRowContext(ctx, tx.Rebind(`SELECT identity,header,stdout_start,stdout_end,stderr_start,stderr_end,last_sequence FROM task_message_shell_output WHERE message_id=?`), messageID).Scan(&head.Identity, &head.Header, &head.StdoutStart, &head.StdoutEnd, &head.StderrStart, &head.StderrEnd, &head.Sequence)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return head, err
}

func shellOutputStateTx(ctx context.Context, tx *sqlx.Tx, messageID string, head *shellOutputHead) (*streams.ShellOutputState, error) {
	if head == nil {
		return nil, nil
	}
	var header streams.ShellOutputPatch
	if err := json.Unmarshal([]byte(head.Header), &header); err != nil {
		return nil, err
	}
	stdout, err := readShellOutputField(ctx, tx, messageID, "stdout", head.StdoutStart, head.StdoutEnd)
	if err != nil {
		return nil, err
	}
	stderr, err := readShellOutputField(ctx, tx, messageID, "stderr", head.StderrStart, head.StderrEnd)
	if err != nil {
		return nil, err
	}
	output := streams.ShellExecOutput{Stdout: stdout, Stderr: stderr, ExitCode: header.ExitCode, Truncated: header.Truncated, StdoutTruncated: header.StdoutTruncated, StderrTruncated: header.StderrTruncated}
	if streams.ShellOutputDigest(output) != header.ResultDigest {
		return nil, streams.ErrShellOutputBaseline
	}
	return &streams.ShellOutputState{Epoch: header.Epoch, Revision: header.Revision, Output: output}, nil
}

func readShellOutputField(ctx context.Context, tx *sqlx.Tx, messageID, field string, start, end int64) (string, error) {
	if start < 0 || end < start || end-start > streams.MaxShellOutputFieldBytes {
		return "", streams.ErrShellOutputBaseline
	}
	if end == start {
		return "", nil
	}
	rows, err := tx.QueryContext(ctx, tx.Rebind(`SELECT ordinal,content FROM task_message_shell_output_chunks WHERE message_id=? AND field=? ORDER BY ordinal`), messageID, field)
	if err != nil {
		return "", err
	}
	defer func() { _ = rows.Close() }()
	output := make([]byte, 0, end-start)
	expected := start / shellOutputChunkBytes
	for rows.Next() {
		var ordinal int64
		var content []byte
		if err := rows.Scan(&ordinal, &content); err != nil {
			return "", err
		}
		if ordinal != expected || len(content) > shellOutputChunkBytes {
			return "", streams.ErrShellOutputBaseline
		}
		offset := int64(0)
		if ordinal == start/shellOutputChunkBytes {
			offset = start % shellOutputChunkBytes
		}
		if offset > int64(len(content)) {
			return "", streams.ErrShellOutputBaseline
		}
		output = append(output, content[offset:]...)
		if len(output) > streams.MaxShellOutputFieldBytes {
			return "", streams.ErrShellOutputBaseline
		}
		expected++
	}
	if err := rows.Err(); err != nil {
		return "", err
	}
	if int64(len(output)) != end-start {
		return "", streams.ErrShellOutputBaseline
	}
	return string(output), nil
}

func applyShellOutputChunksTx(ctx context.Context, tx *sqlx.Tx, messageID, field string, start, end int64, edit *streams.ShellOutputEdit) (int64, int64, error) {
	if edit == nil {
		return start, end, nil
	}
	if edit.Operation == "replace" {
		if _, err := tx.ExecContext(ctx, tx.Rebind(`DELETE FROM task_message_shell_output_chunks WHERE message_id=? AND field=?`), messageID, field); err != nil {
			return 0, 0, err
		}
		start, end = 0, 0
	} else {
		start += int64(edit.Drop)
	}
	if end > math.MaxInt64-int64(len(edit.Data))-shellOutputChunkBytes {
		return 0, 0, streams.ErrShellOutputBaseline
	}
	if _, err := tx.ExecContext(ctx, tx.Rebind(`DELETE FROM task_message_shell_output_chunks WHERE message_id=? AND field=? AND ordinal<?`), messageID, field, start/shellOutputChunkBytes); err != nil {
		return 0, 0, err
	}
	nextEnd, err := appendShellOutputChunksTx(ctx, tx, messageID, field, end, edit.Data)
	return start, nextEnd, err
}

func appendShellOutputChunksTx(ctx context.Context, tx *sqlx.Tx, messageID, field string, end int64, data []byte) (int64, error) {
	for len(data) > 0 {
		ordinal := end / shellOutputChunkBytes
		var content []byte
		if end%shellOutputChunkBytes != 0 {
			err := tx.QueryRowContext(ctx, tx.Rebind(`SELECT content FROM task_message_shell_output_chunks WHERE message_id=? AND field=? AND ordinal=?`), messageID, field, ordinal).Scan(&content)
			if err != nil {
				return 0, err
			}
			if len(content) != int(end%shellOutputChunkBytes) {
				return 0, streams.ErrShellOutputBaseline
			}
		}
		count := min(shellOutputChunkBytes-len(content), len(data))
		content = append(content, data[:count]...)
		_, err := tx.ExecContext(ctx, tx.Rebind(`INSERT INTO task_message_shell_output_chunks(message_id,field,ordinal,content) VALUES(?,?,?,?) ON CONFLICT(message_id,field,ordinal) DO UPDATE SET content=excluded.content`), messageID, field, ordinal, content)
		if err != nil {
			return 0, err
		}
		end += int64(count)
		data = data[count:]
	}
	return end, nil
}

func saveShellOutputHead(ctx context.Context, tx *sqlx.Tx, messageID string, head *shellOutputHead) error {
	_, err := tx.ExecContext(ctx, tx.Rebind(`INSERT INTO task_message_shell_output(message_id,identity,header,stdout_start,stdout_end,stderr_start,stderr_end,last_sequence) VALUES(?,?,?,?,?,?,?,?) ON CONFLICT(message_id) DO UPDATE SET identity=excluded.identity,header=excluded.header,stdout_start=excluded.stdout_start,stdout_end=excluded.stdout_end,stderr_start=excluded.stderr_start,stderr_end=excluded.stderr_end,last_sequence=excluded.last_sequence`), messageID, head.Identity, head.Header, head.StdoutStart, head.StdoutEnd, head.StderrStart, head.StderrEnd, head.Sequence)
	return err
}

func shellOutputHeader(patch *streams.ShellOutputPatch, output streams.ShellExecOutput) (string, error) {
	header := *patch
	header.ExitCode = output.ExitCode
	header.ExitCodePresent = true
	header.Stdout = nil
	header.Stderr = nil
	encoded, err := json.Marshal(header)
	if err != nil {
		return "", fmt.Errorf("encode shell output header: %w", err)
	}
	return string(encoded), nil
}

// RemoveMessageShellOutputTx removes canonical chunk storage with its message tombstone.
func RemoveMessageShellOutputTx(ctx context.Context, tx *sqlx.Tx, messageID string) error {
	_, err := tx.ExecContext(ctx, tx.Rebind(`DELETE FROM task_message_shell_output WHERE message_id=?`), messageID)
	return err
}
