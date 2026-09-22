package corporate

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// An unset HashType must stay out of the /create payload: Mono then
// applies its own default (Gost), so the SDK must not force a value.
func TestDocument_MarshalJSON_omitsEmptyHashType(t *testing.T) {
	body, err := json.Marshal(&SignatureCreateRequest{
		Documents: []Document{{Name: "Договір", Hash: "A4"}},
	})
	require.NoError(t, err)

	assert.NotContains(t, string(body), "hashType")
	assert.Contains(t, string(body), `"hash":"A4"`)
}

func TestDocument_MarshalJSON_roundTripHashType(t *testing.T) {
	in := Document{Name: "Акт", Hash: "BEEF", HashType: HashDstu256, Type: "pdf"}

	body, err := json.Marshal(in)
	require.NoError(t, err)
	assert.Contains(t, string(body), `"hashType":"Dstu256"`)

	var out Document
	require.NoError(t, json.Unmarshal(body, &out))
	assert.Equal(t, in, out)
}

// hashType also comes back in /status responses, through the same
// Document type.
func TestDocument_UnmarshalJSON_hashTypeFromStatus(t *testing.T) {
	in := []byte(`{"documents":[
		{"name":"d1","hash":"A","hashType":"Gost","status":"signed"},
		{"name":"d2","hash":"B","hashType":"Dstu256","status":"pending"},
		{"name":"d3","hash":"C","status":"pending"}
	]}`)

	var resp SignatureStatusResponse
	require.NoError(t, json.Unmarshal(in, &resp))

	require.Len(t, resp.Documents, 3)
	assert.Equal(t, HashGost, resp.Documents[0].HashType)
	assert.Equal(t, HashDstu256, resp.Documents[1].HashType)
	// Omitted by the bank — the caller reads it as the Gost default.
	assert.Empty(t, resp.Documents[2].HashType)
}
