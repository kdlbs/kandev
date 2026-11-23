package pluginsdk

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

func TestSidebarGroupingSnapshotDecoder(t *testing.T) {
	valid := `{"version":1,"revision":"r1","complete":true,"groups":[{"id":"tag:a","label":"Alpha","color":"#aabbcc"}],"memberships":[{"task_id":"task-a","group_id":"tag:a"},{"task_id":"task-a","group_id":"tag:a"}]}`
	got, err := DecodeSidebarGroupingSnapshot([]byte(valid))
	if err != nil {
		t.Fatal(err)
	}
	if got.Version != 1 || got.Revision != "r1" || !got.Complete || !reflect.DeepEqual(got.Memberships, []SidebarGroupingMembership{{TaskID: "task-a", GroupID: "tag:a"}}) {
		t.Fatalf("snapshot = %+v", got)
	}
	cases := map[string]string{
		"incomplete":            strings.Replace(valid, `"complete":true`, `"complete":false`, 1),
		"missing completeness":  strings.Replace(valid, `"complete":true,`, ``, 1),
		"unknown group":         strings.Replace(valid, `"group_id":"tag:a"`, `"group_id":"unknown"`, 1),
		"unsupported version":   strings.Replace(valid, `"version":1`, `"version":2`, 1),
		"empty revision":        strings.Replace(valid, `"r1"`, `""`, 1),
		"invalid color":         strings.Replace(valid, `#aabbcc`, `url(secret)`, 1),
		"invalid UTF-8":         strings.Replace(valid, "Alpha", string([]byte{0xff}), 1),
		"empty task":            strings.Replace(valid, `"task-a"`, `""`, 1),
		"duplicate definitions": strings.Replace(valid, `"groups":[`, `"groups":[{"id":"tag:a","label":"Other"},`, 1),
		"missing groups":        `{"version":1,"revision":"r1","complete":true,"memberships":[]}`,
		"missing memberships":   `{"version":1,"revision":"r1","complete":true,"groups":[]}`,
		"oversized":             strings.Repeat(" ", (1<<20)+1),
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := DecodeSidebarGroupingSnapshot([]byte(body)); err == nil {
				t.Fatal("accepted invalid complete grouping")
			}
		})
	}
	for _, kind := range []string{"groups", "memberships"} {
		t.Run("excess "+kind, func(t *testing.T) {
			payload := map[string]any{"version": 1, "revision": "r", "complete": true, "groups": []any{}, "memberships": []any{}}
			if kind == "groups" {
				groups := make([]map[string]string, 1001)
				for i := range groups {
					groups[i] = map[string]string{"id": fmt.Sprint(i), "label": "Value"}
				}
				payload[kind] = groups
			} else {
				payload["groups"] = []map[string]string{{"id": "g", "label": "Value"}}
				pairs := make([]map[string]string, 10001)
				for i := range pairs {
					pairs[i] = map[string]string{"task_id": fmt.Sprint(i), "group_id": "g"}
				}
				payload[kind] = pairs
			}
			body, err := json.Marshal(payload)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := DecodeSidebarGroupingSnapshot(body); err == nil {
				t.Fatal("accepted excessive provider input")
			}
		})
	}
}

func TestSidebarGroupingSnapshotRejectsAmbiguousFields(t *testing.T) {
	valid := `{"version":1,"revision":"r","complete":true,"groups":[{"id":"g","label":"G"}],"memberships":[]}`
	pair := `{"task_id":"t","group_id":"g"}`
	over := strings.Repeat(pair+",", MaxSidebarGroupingMemberships) + pair
	cases := map[string]string{
		"duplicate memberships":         strings.Replace(valid, `"memberships":[]`, `"memberships":[`+pair+`],"memberships":[]`, 1),
		"folded memberships":            strings.Replace(valid, `"memberships":[]`, `"memberships":[`+pair+`],"MemberShips":[]`, 1),
		"hidden raw overflow":           strings.Replace(valid, `"memberships":[]`, `"memberships":[`+over+`],"memberships":[]`, 1),
		"folded raw overflow":           strings.Replace(valid, `"memberships":[]`, `"memberships":[`+over+`],"MEMBERSHIPS":[]`, 1),
		"duplicate groups":              strings.Replace(valid, `"memberships":[]`, `"groups":[],"memberships":[]`, 1),
		"folded groups":                 strings.Replace(valid, `"memberships":[]`, `"Groups":[],"memberships":[]`, 1),
		"duplicate version":             strings.Replace(valid, `"version":1`, `"version":2,"version":1`, 1),
		"duplicate completeness":        strings.Replace(valid, `"complete":true`, `"complete":false,"complete":true`, 1),
		"duplicate revision":            strings.Replace(valid, `"revision":"r"`, `"revision":"old","revision":"r"`, 1),
		"folded group identity":         strings.Replace(valid, `"id":"g"`, `"id":"other","ID":"g"`, 1),
		"duplicate label":               strings.Replace(valid, `"label":"G"`, `"label":"Old","label":"G"`, 1),
		"folded optional color":         strings.Replace(valid, `"label":"G"`, `"label":"G","Color":"#aabbcc"`, 1),
		"duplicate membership identity": strings.Replace(valid, `"memberships":[]`, `"memberships":[{"task_id":"old","task_id":"t","group_id":"g"}]`, 1),
		"folded membership group":       strings.Replace(valid, `"memberships":[]`, `"memberships":[{"task_id":"t","group_id":"other","GROUP_ID":"g"}]`, 1),
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			got, err := DecodeSidebarGroupingSnapshot([]byte(body))
			if err == nil || got != nil {
				t.Fatal("ambiguous classification admitted as complete")
			}
		})
	}
	t.Run("unrelated extension fields", func(t *testing.T) {
		body := strings.TrimSuffix(valid, "}") + `,"extension":{"nested":[{"x":1,"x":2}],"large":1e99999},"extension":null}`
		got, err := DecodeSidebarGroupingSnapshot([]byte(body))
		if err != nil || got == nil || len(got.Groups) != 1 {
			t.Fatalf("unrelated metadata changed classification: %v", err)
		}
	})
}
