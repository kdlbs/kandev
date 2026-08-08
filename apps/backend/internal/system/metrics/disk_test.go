package metrics

import "testing"

func TestDiskCapacityFromBytesClampsAvailableSpace(t *testing.T) {
	capacity, err := diskCapacityFromBytes(100, 125)
	if err != nil {
		t.Fatalf("diskCapacityFromBytes: %v", err)
	}
	if capacity.TotalBytes != 100 || capacity.AvailableBytes != 100 || capacity.UsedBytes != 0 || capacity.UsedPercent != 0 {
		t.Fatalf("capacity = %#v, want zero used space", capacity)
	}
}

func TestDiskCapacityFromBytesReturnsExactFields(t *testing.T) {
	capacity, err := diskCapacityFromBytes(1000, 250)
	if err != nil {
		t.Fatalf("diskCapacityFromBytes: %v", err)
	}
	if capacity.TotalBytes != 1000 || capacity.AvailableBytes != 250 || capacity.UsedBytes != 750 || capacity.UsedPercent != 75 {
		t.Fatalf("capacity = %#v, want total=1000 available=250 used=750 percent=75", capacity)
	}
}

func TestDiskCapacityFromBytesRejectsZeroTotal(t *testing.T) {
	capacity, err := diskCapacityFromBytes(0, 0)
	if err == nil {
		t.Fatal("diskCapacityFromBytes error = nil, want zero-total error")
	}
	if capacity != (DiskCapacity{}) {
		t.Fatalf("capacity = %#v, want zero value", capacity)
	}
}
