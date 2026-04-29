package tesedi

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const sampleAssetsPage = `{
  "data": [
    {
      "assetName": "HPE 1600W FS Plat",
      "productSku": "830272-B21",
      "serialNumber": "5XLNT0JHLEI0LS",
      "serviceGroupSku": "H7J35AC",
      "serviceGroup": "HPE Foundation Care 24x7 w DMR SVC",
      "serviceLevels": [
        {"serviceLevelSku": "HA151AC", "serviceLevel": "HPE Hardware Maintenance", "retailPrice": 0}
      ],
      "retailPrice": null,
      "startDate": "2021-04-29",
      "endDate": "2026-04-30"
    }
  ],
  "meta": {"hasNextPage": true, "nextCursor": "3"}
}`

func TestSearchContracts_PassesQueryParams(t *testing.T) {
	var capturedQuery string
	mux := http.NewServeMux()
	mux.HandleFunc("/auth", validAuthHandler(t, "k", nil))
	mux.HandleFunc("/api/contracts", func(w http.ResponseWriter, r *http.Request) {
		capturedQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[],"meta":{"hasNextPage":false,"nextCursor":""}}`))
	})
	server := startServer(t, mux)

	c := New(server.URL+"/api", server.URL+"/auth", "k", WithRetry(1, 0))
	_, err := c.SearchContracts(context.Background(), "abc", &ListOptions{
		Limit:     25,
		Cursor:    "X:Y",
		SortBy:    "contractNumber",
		SortOrder: "desc",
	})
	require.NoError(t, err)

	assert.Contains(t, capturedQuery, "search=abc")
	assert.Contains(t, capturedQuery, "limit=25")
	assert.Contains(t, capturedQuery, "cursor=X%3AY")
	assert.Contains(t, capturedQuery, "sortBy=contractNumber")
	assert.Contains(t, capturedQuery, "sortOrder=desc")
}

func TestSearchContracts_EmptyQueryOmitsSearchParam(t *testing.T) {
	var capturedQuery string
	mux := http.NewServeMux()
	mux.HandleFunc("/auth", validAuthHandler(t, "k", nil))
	mux.HandleFunc("/api/contracts", func(w http.ResponseWriter, r *http.Request) {
		capturedQuery = r.URL.RawQuery
		_, _ = w.Write([]byte(`{"data":[],"meta":{"hasNextPage":false,"nextCursor":""}}`))
	})
	server := startServer(t, mux)

	c := New(server.URL+"/api", server.URL+"/auth", "k", WithRetry(1, 0))
	_, err := c.SearchContracts(context.Background(), "", nil)
	require.NoError(t, err)
	assert.NotContains(t, capturedQuery, "search=")
}

func TestSearchContracts_DecodesPage(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/auth", validAuthHandler(t, "k", nil))
	mux.HandleFunc("/api/contracts", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
            "data": [{
                "contractId": "15683",
                "contractNumber": "2137277728",
                "status": "Active",
                "vendor": "HPE",
                "endDate": "2026-04-30"
            }],
            "meta": {"hasNextPage": true, "nextCursor": "2026-04-30:15683"}
        }`))
	})
	server := startServer(t, mux)

	c := New(server.URL+"/api", server.URL+"/auth", "k", WithRetry(1, 0))
	page, err := c.SearchContracts(context.Background(), "any", nil)
	require.NoError(t, err)
	require.Len(t, page.Data, 1)
	assert.Equal(t, "15683", page.Data[0].ContractID)
	assert.Equal(t, "2137277728", page.Data[0].ContractNumber)
	assert.Equal(t, "HPE", page.Data[0].Vendor)
	assert.True(t, page.Meta.HasNextPage)
	assert.Equal(t, "2026-04-30:15683", page.Meta.NextCursor)
}

func TestGetContractByNumber_ExactMatch(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/auth", validAuthHandler(t, "k", nil))
	mux.HandleFunc("/api/contracts", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{
            "data": [
                {"contractId":"1","contractNumber":"21372777"},
                {"contractId":"2","contractNumber":"2137277728"},
                {"contractId":"3","contractNumber":"2137277728X"}
            ],
            "meta":{"hasNextPage":false,"nextCursor":""}
        }`))
	})
	server := startServer(t, mux)

	c := New(server.URL+"/api", server.URL+"/auth", "k", WithRetry(1, 0))
	got, err := c.GetContractByNumber(context.Background(), "2137277728")
	require.NoError(t, err)
	assert.Equal(t, "2", got.ContractID)
}

func TestGetContractByNumber_NotFound(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/auth", validAuthHandler(t, "k", nil))
	mux.HandleFunc("/api/contracts", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":[],"meta":{"hasNextPage":false,"nextCursor":""}}`))
	})
	server := startServer(t, mux)

	c := New(server.URL+"/api", server.URL+"/auth", "k", WithRetry(1, 0))
	_, err := c.GetContractByNumber(context.Background(), "missing")
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrContractNotFound))
}

func TestGetContractByNumber_NoExactMatchInFuzzyResults(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/auth", validAuthHandler(t, "k", nil))
	mux.HandleFunc("/api/contracts", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{
            "data": [
                {"contractId":"1","contractNumber":"abc-123"},
                {"contractId":"2","contractNumber":"abc-1234"}
            ],
            "meta":{"hasNextPage":false,"nextCursor":""}
        }`))
	})
	server := startServer(t, mux)

	c := New(server.URL+"/api", server.URL+"/auth", "k", WithRetry(1, 0))
	_, err := c.GetContractByNumber(context.Background(), "abc-12")
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrContractNotFound))
}

func TestGetContractByNumber_Ambiguous(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/auth", validAuthHandler(t, "k", nil))
	mux.HandleFunc("/api/contracts", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{
            "data": [
                {"contractId":"1","contractNumber":"DUPLICATE"},
                {"contractId":"2","contractNumber":"DUPLICATE"}
            ],
            "meta":{"hasNextPage":false,"nextCursor":""}
        }`))
	})
	server := startServer(t, mux)

	c := New(server.URL+"/api", server.URL+"/auth", "k", WithRetry(1, 0))
	_, err := c.GetContractByNumber(context.Background(), "DUPLICATE")
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrAmbiguousContractNumber))
}

func TestGetContract_DecodesWrapper(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/auth", validAuthHandler(t, "k", nil))
	mux.HandleFunc("/api/contracts/15683", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{
            "data": {
                "contractId": "15683",
                "contractNumber": "2137277728",
                "status": "Active",
                "endDate": "2026-04-30"
            }
        }`))
	})
	server := startServer(t, mux)

	c := New(server.URL+"/api", server.URL+"/auth", "k", WithRetry(1, 0))
	got, err := c.GetContract(context.Background(), "15683")
	require.NoError(t, err)
	assert.Equal(t, "15683", got.ContractID)
	assert.Equal(t, "2137277728", got.ContractNumber)
	assert.Equal(t, "Active", got.Status)
	assert.Empty(t, got.Vendor, "GetContract response is documented not to include vendor")
}

func TestGetContract_PathEscape(t *testing.T) {
	var capturedURI string
	mux := http.NewServeMux()
	mux.HandleFunc("/auth", validAuthHandler(t, "k", nil))
	mux.Handle("/api/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// RequestURI is the raw path as received on the wire, before Go's URL
		// decoding rewrites %2F into '/' on r.URL.Path.
		capturedURI = r.RequestURI
		_, _ = w.Write([]byte(`{"data":{"contractId":"x"}}`))
	}))
	server := startServer(t, mux)

	c := New(server.URL+"/api", server.URL+"/auth", "k", WithRetry(1, 0))
	_, err := c.GetContract(context.Background(), "abc/def")
	require.NoError(t, err)
	assert.Equal(t, "/api/contracts/abc%2Fdef", capturedURI)
}

func TestGetContractAssets_DecodesAssets(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/auth", validAuthHandler(t, "k", nil))
	mux.HandleFunc("/api/contracts/15683/assets", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(sampleAssetsPage))
	})
	server := startServer(t, mux)

	c := New(server.URL+"/api", server.URL+"/auth", "k", WithRetry(1, 0))
	page, err := c.GetContractAssets(context.Background(), "15683", nil)
	require.NoError(t, err)

	require.Len(t, page.Data, 1)
	a := page.Data[0]
	assert.Equal(t, "830272-B21", a.ProductSKU)
	assert.Equal(t, "5XLNT0JHLEI0LS", a.SerialNumber)
	assert.Equal(t, "HPE Foundation Care 24x7 w DMR SVC", a.ServiceGroup)
	require.Len(t, a.ServiceLevels, 1)
	assert.Equal(t, "HA151AC", a.ServiceLevels[0].ServiceLevelSKU)
	assert.Nil(t, a.RetailPrice)
	assert.Equal(t, "2026-04-30", a.EndDate)

	assert.True(t, page.Meta.HasNextPage)
	assert.Equal(t, "3", page.Meta.NextCursor)
}

func TestGetContractAssets_PassesPagination(t *testing.T) {
	var capturedQuery string
	mux := http.NewServeMux()
	mux.HandleFunc("/auth", validAuthHandler(t, "k", nil))
	mux.HandleFunc("/api/contracts/1/assets", func(w http.ResponseWriter, r *http.Request) {
		capturedQuery = r.URL.RawQuery
		_, _ = w.Write([]byte(`{"data":[],"meta":{"hasNextPage":false,"nextCursor":""}}`))
	})
	server := startServer(t, mux)

	c := New(server.URL+"/api", server.URL+"/auth", "k", WithRetry(1, 0))
	_, err := c.GetContractAssets(context.Background(), "1", &ListOptions{Limit: 10, Cursor: "abc"})
	require.NoError(t, err)
	assert.Contains(t, capturedQuery, "limit=10")
	assert.Contains(t, capturedQuery, "cursor=abc")
}

// Sanity: a default context shouldn't time out the test suite if the upstream
// timeout cap on retries isn't honored.
func TestSearchContracts_RetryRespectsContextDeadline(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/auth", validAuthHandler(t, "k", nil))
	mux.HandleFunc("/api/contracts", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	})
	server := startServer(t, mux)

	c := New(server.URL+"/api", server.URL+"/auth", "k", WithRetry(20, 100*time.Millisecond))
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	_, err := c.SearchContracts(ctx, "anything", nil)
	require.Error(t, err)
}
