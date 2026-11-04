package pluginsdk_test

import (
	"testing"

	"github.com/kdlbs/kandev/pluginsdk"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

func TestCompiledSandboxServiceDeclarationRoundTrip(t *testing.T) {
	const body = `{"approvalServicesV2":[{"serviceKey":"network-proxy","routeId":"connect","operationKind":"network","endpointReference":"sandbox.approval.service.network-proxy.connect.endpoint"}]}`
	var compiled pluginsdk.SandboxCompiledPolicy
	if err := protojson.Unmarshal([]byte(body), &compiled); err != nil {
		t.Fatalf("decode the compiled service declaration: %v", err)
	}
	field := compiled.ProtoReflect().Descriptor().Fields().ByName("approval_services_v2")
	if field == nil || field.Number() != 101 {
		t.Fatal("service declarations must retain field number 101")
	}
	expected := &pluginsdk.SandboxApprovalServiceV2{ServiceKey: "network-proxy", RouteId: "connect", OperationKind: "network", EndpointReference: "sandbox.approval.service.network-proxy.connect.endpoint"}
	if len(compiled.ApprovalServicesV2) != 1 || !proto.Equal(expected, compiled.ApprovalServicesV2[0]) {
		t.Fatal("the public SDK declaration differs from its JSON contract")
	}
	raw, err := proto.Marshal(&compiled)
	if err != nil {
		t.Fatal(err)
	}
	var restored pluginsdk.SandboxCompiledPolicy
	if err = proto.Unmarshal(raw, &restored); err != nil {
		t.Fatal(err)
	}
	if !proto.Equal(&compiled, &restored) {
		t.Fatal("the binary wire round trip changed the service declaration")
	}
}
