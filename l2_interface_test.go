package aoscxgo

import (
	"net/http"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// portSwitch is a fake switch with the VLANs in vlans and one port, 1/1/1,
// answering port as its attributes. It takes every PATCH.
func portSwitch(t *testing.T, vlans []int, port string) *fakeSwitch {
	known := map[string]bool{}
	for _, id := range vlans {
		known["/rest/v10.16/system/vlans/"+strconv.Itoa(id)] = true
	}
	return newFakeSwitch(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "GET" && known[r.URL.Path]:
			w.Write([]byte(`{"admin": null, "name": "a VLAN"}`))
		case r.Method == "GET" && r.URL.EscapedPath() == "/rest/v10.16/system/interfaces/1%2F1%2F1":
			w.Write([]byte(port))
		case r.Method == "PATCH":
			w.WriteHeader(http.StatusNoContent)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})
}

// checkSent fails unless the fake was sent exactly want, each as its method
// and URI, and for a PATCH its JSON body.
func checkSent(t *testing.T, fake *fakeSwitch, want []request) {
	t.Helper()
	sent := fake.sent()
	if len(sent) != len(want) {
		t.Fatalf("sent %d requests, not %d:\n got %v\nwant %v", len(sent), len(want), sent, want)
	}
	for i := range want {
		if sent[i].Method != want[i].Method || sent[i].URI != want[i].URI {
			t.Errorf("request %d is %s %s, not %s %s", i, sent[i].Method, sent[i].URI, want[i].Method, want[i].URI)
			continue
		}
		if want[i].Body != "" && !sameJSON(t, sent[i].Body, want[i].Body) {
			t.Errorf("%s %s sent\n %s\nnot\n %s", sent[i].Method, sent[i].URI, sent[i].Body, want[i].Body)
		}
	}
}

const port1 = "/rest/v10.16/system/interfaces/1%2F1%2F1"

func TestInterfaceCreateSendsNothing(t *testing.T) {
	fake := portSwitch(t, nil, `{}`)
	if err := (&Interface{Name: "1/1/1", AdminState: "up"}).Create(fake.client()); err != nil {
		t.Fatalf("Create: %v", err)
	}
	checkSent(t, fake, nil)
}

func TestInterfaceUpdatePatchesTheDescriptionAlone(t *testing.T) {
	fake := portSwitch(t, nil, `{}`)
	port := Interface{Name: "1/1/1", Description: "uplink", AdminState: "down"}
	if err := port.Update(fake.client()); err != nil {
		t.Fatalf("Update: %v", err)
	}
	checkSent(t, fake, []request{{Method: "PATCH", URI: port1, Body: `{"description": "uplink"}`}})
}

func TestL2InterfaceAccessPatchesItsVlansAlone(t *testing.T) {
	fake := portSwitch(t, []int{10}, `{}`)
	port := L2Interface{Interface: Interface{Name: "1/1/1", Description: "kept", AdminState: "up"}, Description: "kept", VlanMode: "access", VlanTag: 10}
	if err := port.Create(fake.client()); err != nil {
		t.Fatalf("Create: %v", err)
	}
	checkSent(t, fake, []request{
		{Method: "GET", URI: "/rest/v10.16/system/vlans/10"},
		{Method: "PATCH", URI: port1, Body: `{"vlan_mode": "access", "vlan_tag": {"10": "/rest/v10.16/system/vlans/10"}, "vlan_trunks": {}}`},
	})
}

func TestL2InterfaceTrunkNamesItsNativeVlanAndAllowedVlans(t *testing.T) {
	fake := portSwitch(t, []int{1, 20}, `{}`)
	port := L2Interface{Interface: Interface{Name: "1/1/1"}, VlanMode: "trunk", VlanTag: 1, VlanIds: []interface{}{20, 1}}
	if err := port.Update(fake.client()); err != nil {
		t.Fatalf("Update: %v", err)
	}
	checkSent(t, fake, []request{
		{Method: "GET", URI: "/rest/v10.16/system/vlans/1"},
		{Method: "GET", URI: "/rest/v10.16/system/vlans/20"},
		{Method: "GET", URI: "/rest/v10.16/system/vlans/1"},
		{Method: "PATCH", URI: port1, Body: `{"vlan_mode": "native-untagged", "vlan_tag": {"1": "/rest/v10.16/system/vlans/1"}, ` +
			`"vlan_trunks": {"1": "/rest/v10.16/system/vlans/1", "20": "/rest/v10.16/system/vlans/20"}}`},
	})
}

func TestL2InterfaceTrunkAllowingAllSendsNoTrunks(t *testing.T) {
	fake := portSwitch(t, []int{5}, `{}`)
	port := L2Interface{Interface: Interface{Name: "1/1/1"}, VlanMode: "trunk", VlanTag: 5, TrunkAllowedAll: true, NativeVlanTag: true}
	if err := port.Update(fake.client()); err != nil {
		t.Fatalf("Update: %v", err)
	}
	checkSent(t, fake, []request{
		{Method: "GET", URI: "/rest/v10.16/system/vlans/5"},
		{Method: "PATCH", URI: port1, Body: `{"vlan_mode": "native-tagged", "vlan_tag": {"5": "/rest/v10.16/system/vlans/5"}, "vlan_trunks": {}}`},
	})
}

func TestL2InterfaceRefusesWithoutPatching(t *testing.T) {
	for _, c := range []struct {
		name    string
		vlans   []int
		port    L2Interface
		refusal string
	}{
		{"a VLAN the switch lacks", []int{1}, L2Interface{VlanMode: "trunk", VlanTag: 1, VlanIds: []interface{}{1, 30}}, "VLAN 30 could not be read"},
		{"a missing access VLAN", nil, L2Interface{VlanMode: "access", VlanTag: 30}, "VLAN 30 could not be read"},
		{"a native VLAN not allowed", []int{1, 20}, L2Interface{VlanMode: "trunk", VlanTag: 1, VlanIds: []interface{}{20}}, "native VLAN 1 is not among"},
		{"no allowed VLANs", []int{1}, L2Interface{VlanMode: "trunk", VlanTag: 1}, "lists its allowed VLANs"},
		{"both allowed VLANs and all", []int{1}, L2Interface{VlanMode: "trunk", VlanTag: 1, VlanIds: []interface{}{1}, TrunkAllowedAll: true}, "not both"},
	} {
		t.Run(c.name, func(t *testing.T) {
			fake := portSwitch(t, c.vlans, `{}`)
			c.port.Interface.Name = "1/1/1"
			err := c.port.Update(fake.client())
			if err == nil {
				t.Fatalf("Update succeeded")
			}
			if status := err.(*RequestError).StatusCode; !strings.Contains(status, c.refusal) {
				t.Errorf("refused with %q, not %q", status, c.refusal)
			}
			for _, sent := range fake.sent() {
				if sent.Method != "GET" {
					t.Errorf("sent %s %s", sent.Method, sent.URI)
				}
			}
		})
	}
}

func TestL2InterfaceGetReadsNullVlans(t *testing.T) {
	fake := portSwitch(t, nil, `{"vlan_mode": "native-untagged", "vlan_tag": null, "vlan_trunks": null, "admin": null, "description": null}`)
	port := L2Interface{Interface: Interface{Name: "1/1/1"}}
	if err := port.Get(fake.client()); err != nil {
		t.Fatalf("Get: %v", err)
	}
	if port.VlanTag != 0 || len(port.VlanIds) != 0 || !port.TrunkAllowedAll {
		t.Errorf("read %+v, not a trunk allowing every VLAN with no native VLAN", port)
	}
	checkSent(t, fake, []request{{Method: "GET", URI: port1 + "?selector=writable"}})
}

func TestL2InterfaceGetReadsTrunkVlans(t *testing.T) {
	fake := portSwitch(t, nil, `{"vlan_mode": "native-tagged", "vlan_tag": {"5": "/rest/v10.16/system/vlans/5"}, `+
		`"vlan_trunks": {"5": "/rest/v10.16/system/vlans/5", "7": "/rest/v10.16/system/vlans/7"}}`)
	port := L2Interface{Interface: Interface{Name: "1/1/1"}}
	if err := port.Get(fake.client()); err != nil {
		t.Fatalf("Get: %v", err)
	}
	ids := []int{}
	for _, id := range port.VlanIds {
		ids = append(ids, id.(int))
	}
	sort.Ints(ids)
	if port.VlanTag != 5 || !port.NativeVlanTag || port.TrunkAllowedAll || !reflect.DeepEqual(ids, []int{5, 7}) {
		t.Errorf("read %+v", port)
	}
}

func TestVlanGetReadsANullAdmin(t *testing.T) {
	fake := portSwitch(t, []int{10}, `{}`)
	vlan := Vlan{VlanId: 10}
	if err := vlan.Get(fake.client()); err != nil {
		t.Fatalf("Get: %v", err)
	}
	if vlan.Name != "a VLAN" || vlan.AdminState != "" || vlan.GetURI() != "/rest/v10.16/system/vlans/10" {
		t.Errorf("read %+v", vlan)
	}
}
