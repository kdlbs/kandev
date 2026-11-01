package journal

const CurrentProtocolVersion uint32 = 2
const DeliveryProtocolHeader = "X-Kandev-Delivery-Version"

// SupportsDeliveryVersion accepts retained snapshot runtimes and compact-output runtimes.
func SupportsDeliveryVersion(version uint32) bool {
	return version == 1 || version == CurrentProtocolVersion
}
