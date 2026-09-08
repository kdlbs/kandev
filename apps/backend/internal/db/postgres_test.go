package db

import (
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

func TestConfigurePostgresTimeTypesUsesUTCScans(t *testing.T) {
	typeMap := pgtype.NewMap()
	if err := configurePostgresTimeTypes(typeMap); err != nil {
		t.Fatalf("configurePostgresTimeTypes: %v", err)
	}

	timestamp, ok := typeMap.TypeForName("timestamp")
	if !ok {
		t.Fatal("timestamp type is not registered")
	}
	if codec, ok := timestamp.Codec.(*pgtype.TimestampCodec); !ok || codec.ScanLocation != time.UTC {
		t.Fatalf("timestamp scan location = %#v, want UTC", timestamp.Codec)
	}

	timestamptz, ok := typeMap.TypeForName("timestamptz")
	if !ok {
		t.Fatal("timestamptz type is not registered")
	}
	if codec, ok := timestamptz.Codec.(*pgtype.TimestamptzCodec); !ok || codec.ScanLocation != time.UTC {
		t.Fatalf("timestamptz scan location = %#v, want UTC", timestamptz.Codec)
	}
}
