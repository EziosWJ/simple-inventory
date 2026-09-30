//go:build integration

package integration

import (
	"encoding/json"
	"net/http"
	"testing"
)

func assertBusinessDictionaryContract(t *testing.T, router http.Handler, token string) {
	t.Helper()
	expected := map[string]map[string]string{
		"PRODUCT_TYPE":     {"GOODS": "实物", "SERVICE": "服务"},
		"PARTNER_TYPE":     {"COMPANY": "单位", "PERSON": "个人"},
		"PARTNER_IDENTITY": {"CUSTOMER": "客户", "SUPPLIER": "供应商"},
		"BUSINESS_STATUS":  {"1": "启用", "0": "停用"},
	}
	for code, want := range expected {
		response := serveJSON(router, http.MethodGet, "/api/system/dict/"+code+"/items", "", token)
		assertEnvelopeCode(t, response, http.StatusOK, http.StatusOK, "success")
		var envelope struct {
			Data []struct {
				Label string `json:"label"`
				Value string `json:"value"`
			} `json:"data"`
		}
		if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil {
			t.Fatalf("decode %s dictionary response: %v", code, err)
		}
		if len(envelope.Data) != len(want) {
			t.Fatalf("%s dictionary has %d items, want %d: %s", code, len(envelope.Data), len(want), response.Body.String())
		}
		for _, item := range envelope.Data {
			if label, ok := want[item.Value]; !ok || label != item.Label {
				t.Fatalf("%s dictionary item %q=%q, want a seeded value and matching label", code, item.Value, item.Label)
			}
			delete(want, item.Value)
		}
		if len(want) != 0 {
			t.Fatalf("%s dictionary is missing seeded values: %v", code, want)
		}
	}
}
