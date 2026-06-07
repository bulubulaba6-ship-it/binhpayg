package management

import (
	"strings"
	"testing"
)

// TestVerifyPayOSSignature_OfficialSample verifies our implementation against
// the official PayOS documentation sample data at:
// https://payos.vn/docs/tich-hop-webhook/kiem-tra-du-lieu-voi-signature
//
// The JS reference uses checksumKey "1a54716c..." and produces sig "412e915d..."
// The webhook endpoint sample uses the same data but sig "8d8640d..." (different key, not shown).
// We test against the JS reference pair since that key is documented.
//
// Expected query string (sorted alpha):
// accountNumber=12345678&amount=3000&code=00&counterAccountBankId=&counterAccountBankName=&
// counterAccountName=&counterAccountNumber=&currency=VND&desc=Thành công&
// description=VQRIO123&orderCode=123&paymentLinkId=124c33293c43417ab7879e14c8d9eb18&
// reference=TF230204212323&transactionDateTime=2023-02-04 18:25:00&
// virtualAccountName=&virtualAccountNumber=
func TestVerifyPayOSSignature_OfficialSample(t *testing.T) {
	checksumKey := "1a54716c8f0efb2744fb28b6e38b25da7f67a925d98bc1c18bd8faaecadd7675"

	// Exactly as payOS sends it: empty strings for counter/virtual account fields,
	// integer orderCode and amount decoded as float64 by encoding/json.
	data := map[string]interface{}{
		"orderCode":              float64(123),
		"amount":                 float64(3000),
		"description":            "VQRIO123",
		"accountNumber":          "12345678",
		"reference":              "TF230204212323",
		"transactionDateTime":    "2023-02-04 18:25:00",
		"currency":               "VND",
		"paymentLinkId":          "124c33293c43417ab7879e14c8d9eb18",
		"code":                   "00",
		"desc":                   "Thành công",
		"counterAccountBankId":   "",
		"counterAccountBankName": "",
		"counterAccountName":     "",
		"counterAccountNumber":   "",
		"virtualAccountName":     "",
		"virtualAccountNumber":   "",
	}

	// This is the signature from the payOS JS reference sample
	expectedSig := "412e915d2871504ed31be63c8f62a149a4410d34c4c42affc9006ef9917eaa03"

	ok, queryStr := verifyPayOSSignature(data, expectedSig, checksumKey)
	if !ok {
		t.Errorf("signature verification failed\nquery_string=%q\nexpected_sig=%s", queryStr, expectedSig)
	}
}

// TestVerifyPayOSSignature_NullFields verifies that nil (JSON null) values are
// treated as empty string and INCLUDED in the query string, per payOS PHP/JS spec.
func TestVerifyPayOSSignature_NullFields(t *testing.T) {
	checksumKey := "testkey"

	dataNil := map[string]interface{}{
		"amount":      float64(3000),
		"description": "TEST",
		"extra":       nil, // JSON null → must become "extra=" not be omitted
	}

	_, qsNil := verifyPayOSSignature(dataNil, "", checksumKey)
	if !strings.Contains(qsNil, "extra=") {
		t.Errorf("nil field should appear as 'key=' in query string, got: %s", qsNil)
	}
}

// TestVerifyPayOSSignature_NullStringLiterals verifies that "null" and "undefined"
// string values are treated as empty strings per payOS PHP spec.
func TestVerifyPayOSSignature_NullStringLiterals(t *testing.T) {
	checksumKey := "testkey"

	data := map[string]interface{}{
		"amount": float64(100),
		"field1": "null",      // string "null" → ""
		"field2": "undefined", // string "undefined" → ""
		"field3": "",          // empty string → ""
	}

	_, qs := verifyPayOSSignature(data, "", checksumKey)
	if !strings.Contains(qs, "field1=") {
		t.Errorf("field1 should be present as 'field1=', got: %s", qs)
	}
	if !strings.Contains(qs, "field2=") {
		t.Errorf("field2 should be present as 'field2=', got: %s", qs)
	}
	if strings.Contains(qs, "field1=null") {
		t.Errorf("field1 should not have value 'null', got: %s", qs)
	}
}
