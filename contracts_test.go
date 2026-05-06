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
      "assetName": "AcmeVendor 1600W Power Supply",
      "productSku": "FAKE-SKU-001",
      "serialNumber": "SN-TEST-0001",
      "serviceGroupSku": "FAKE-GROUP-AC",
      "serviceGroup": "AcmeVendor Foundation Care 24x7",
      "serviceLevels": [
        {"serviceLevelSku": "FAKE-LEVEL-AC", "serviceLevel": "AcmeVendor Hardware Maintenance", "retailPrice": 0}
      ],
      "retailPrice": null,
      "startDate": "2024-01-01",
      "endDate": "2027-12-31"
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
                "contractId": "99999",
                "contractNumber": "FAKE-CONTRACT-0001",
                "status": "Active",
                "vendor": "AcmeVendor",
                "endDate": "2026-04-30"
            }],
            "meta": {"hasNextPage": true, "nextCursor": "2027-12-31:99999"}
        }`))
	})
	server := startServer(t, mux)

	c := New(server.URL+"/api", server.URL+"/auth", "k", WithRetry(1, 0))
	page, err := c.SearchContracts(context.Background(), "any", nil)
	require.NoError(t, err)
	require.Len(t, page.Data, 1)
	assert.Equal(t, "99999", page.Data[0].ContractID)
	assert.Equal(t, "FAKE-CONTRACT-0001", page.Data[0].ContractNumber)
	assert.Equal(t, "AcmeVendor", page.Data[0].Vendor)
	assert.True(t, time.Date(2026, time.April, 30, 0, 0, 0, 0, time.UTC).Equal(page.Data[0].EndDate.Time))
	assert.True(t, page.Meta.HasNextPage)
	assert.Equal(t, "2027-12-31:99999", page.Meta.NextCursor)
}

func TestGetContractByNumber_ExactMatch(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/auth", validAuthHandler(t, "k", nil))
	mux.HandleFunc("/api/contracts", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{
            "data": [
                {"contractId":"1","contractNumber":"FAKE-CONTRACT-0009"},
                {"contractId":"2","contractNumber":"FAKE-CONTRACT-0001"},
                {"contractId":"3","contractNumber":"FAKE-CONTRACT-0001-X"}
            ],
            "meta":{"hasNextPage":false,"nextCursor":""}
        }`))
	})
	server := startServer(t, mux)

	c := New(server.URL+"/api", server.URL+"/auth", "k", WithRetry(1, 0))
	got, err := c.GetContractByNumber(context.Background(), "FAKE-CONTRACT-0001")
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
	mux.HandleFunc("/api/contracts/99999", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{
            "data": {
                "contractId": "99999",
                "contractNumber": "FAKE-CONTRACT-0001",
                "status": "Active",
                "endDate": "2026-04-30"
            }
        }`))
	})
	server := startServer(t, mux)

	c := New(server.URL+"/api", server.URL+"/auth", "k", WithRetry(1, 0))
	got, err := c.GetContract(context.Background(), "99999")
	require.NoError(t, err)
	assert.Equal(t, "99999", got.ContractID)
	assert.Equal(t, "FAKE-CONTRACT-0001", got.ContractNumber)
	assert.Equal(t, "Active", got.Status)
	assert.True(t, time.Date(2026, time.April, 30, 0, 0, 0, 0, time.UTC).Equal(got.EndDate.Time))
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
	mux.HandleFunc("/api/contracts/99999/assets", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(sampleAssetsPage))
	})
	server := startServer(t, mux)

	c := New(server.URL+"/api", server.URL+"/auth", "k", WithRetry(1, 0))
	page, err := c.GetContractAssets(context.Background(), "99999", nil)
	require.NoError(t, err)

	require.Len(t, page.Data, 1)
	a := page.Data[0]
	assert.Equal(t, "FAKE-SKU-001", a.ProductSKU)
	assert.Equal(t, "SN-TEST-0001", a.SerialNumber)
	assert.Equal(t, "AcmeVendor Foundation Care 24x7", a.ServiceGroup)
	require.Len(t, a.ServiceLevels, 1)
	assert.Equal(t, "FAKE-LEVEL-AC", a.ServiceLevels[0].ServiceLevelSKU)
	assert.Nil(t, a.RetailPrice)
	assert.True(t, time.Date(2024, time.January, 1, 0, 0, 0, 0, time.UTC).Equal(a.StartDate.Time))
	assert.True(t, time.Date(2027, time.December, 31, 0, 0, 0, 0, time.UTC).Equal(a.EndDate.Time))

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
