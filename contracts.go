package tesedi

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
)

// SearchContracts returns a page of contracts matching the query. An empty
// query lists all contracts visible to the partner. Pagination is cursor-based;
// pass ListOptions.Cursor to resume.
//
// Note: the Tesedi /contracts endpoint silently ignores unknown query params,
// so SearchContracts only sets parameters known to be honored upstream.
func (c *Client) SearchContracts(ctx context.Context, query string, opts *ListOptions) (*ContractPage, error) {
	q := buildListQuery(opts)
	if query != "" {
		q.Set("search", query)
	}
	resp, err := c.do(ctx, http.MethodGet, "/contracts", q)
	if err != nil {
		return nil, err
	}
	var page ContractPage
	if err := decode(resp, &page); err != nil {
		return nil, err
	}
	return &page, nil
}

// GetContractByNumber resolves a contract by its contractNumber.
//
// The upstream API does not support exact lookup by contractNumber, so this
// method issues a search and validates that exactly one returned row has a
// contractNumber equal to the requested value. Returns ErrContractNotFound
// if no exact match exists, or ErrAmbiguousContractNumber if more than one
// row matches.
func (c *Client) GetContractByNumber(ctx context.Context, number string) (*Contract, error) {
	page, err := c.SearchContracts(ctx, number, &ListOptions{Limit: 50})
	if err != nil {
		return nil, err
	}
	var match *Contract
	for i := range page.Data {
		if page.Data[i].ContractNumber == number {
			if match != nil {
				return nil, ErrAmbiguousContractNumber
			}
			match = &page.Data[i]
		}
	}
	if match == nil {
		return nil, ErrContractNotFound
	}
	return match, nil
}

// GetContract fetches a contract by its contractId.
//
// The upstream API does not populate the Vendor field on this response — it
// is only present on list/search responses. Use GetContractByNumber or
// SearchContracts when Vendor is required.
func (c *Client) GetContract(ctx context.Context, contractID string) (*Contract, error) {
	resp, err := c.do(ctx, http.MethodGet, "/contracts/"+url.PathEscape(contractID), nil)
	if err != nil {
		return nil, err
	}
	var wrap struct {
		Data Contract `json:"data"`
	}
	if err := decode(resp, &wrap); err != nil {
		return nil, err
	}
	return &wrap.Data, nil
}

// GetContractAssets returns a page of assets covered by the given contract.
// Pagination is cursor-based; pass ListOptions.Cursor to resume.
func (c *Client) GetContractAssets(ctx context.Context, contractID string, opts *ListOptions) (*AssetPage, error) {
	q := buildListQuery(opts)
	resp, err := c.do(ctx, http.MethodGet, "/contracts/"+url.PathEscape(contractID)+"/assets", q)
	if err != nil {
		return nil, err
	}
	var page AssetPage
	if err := decode(resp, &page); err != nil {
		return nil, err
	}
	return &page, nil
}

func buildListQuery(opts *ListOptions) url.Values {
	q := url.Values{}
	if opts == nil {
		return q
	}
	if opts.Limit > 0 {
		q.Set("limit", strconv.Itoa(opts.Limit))
	}
	if opts.Cursor != "" {
		q.Set("cursor", opts.Cursor)
	}
	if opts.SortBy != "" {
		q.Set("sortBy", opts.SortBy)
	}
	if opts.SortOrder != "" {
		q.Set("sortOrder", opts.SortOrder)
	}
	return q
}
