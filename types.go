package tesedi

// Contract represents a Tesedi service contract.
//
// The Vendor field is populated by list/search responses but omitted by
// GetContract — that is an upstream API asymmetry, not a bug in this client.
type Contract struct {
	ContractID      string  `json:"contractId"`
	ContractNumber  string  `json:"contractNumber"`
	Status          string  `json:"status"`
	GroupID         string  `json:"groupId"`
	SAR             string  `json:"sar"`
	Vendor          string  `json:"vendor,omitempty"`
	Distributor     string  `json:"distributor"`
	Reseller        string  `json:"reseller"`
	EndCustomerName string  `json:"endCustomerName"`
	EndCustomerID   string  `json:"endCustomerId"`
	StartDate       string  `json:"startDate"` // YYYY-MM-DD
	EndDate         string  `json:"endDate"`   // YYYY-MM-DD
	ResellerPrice   float64 `json:"resellerPrice"`
	RetailPrice     float64 `json:"retailPrice"`
	Currency        string  `json:"currency"`
}

// Asset represents an asset covered by a contract.
type Asset struct {
	AssetName       string         `json:"assetName"`
	ProductSKU      string         `json:"productSku"`
	SerialNumber    string         `json:"serialNumber"`
	ServiceGroupSKU string         `json:"serviceGroupSku"`
	ServiceGroup    string         `json:"serviceGroup"`
	ServiceLevels   []ServiceLevel `json:"serviceLevels"`
	RetailPrice     *float64       `json:"retailPrice"`
	StartDate       string         `json:"startDate"`
	EndDate         string         `json:"endDate"`
}

// ServiceLevel describes one component of an asset's service group.
type ServiceLevel struct {
	ServiceLevelSKU string  `json:"serviceLevelSku"`
	ServiceLevel    string  `json:"serviceLevel"`
	RetailPrice     float64 `json:"retailPrice"`
}

// PageMeta carries cursor-based pagination metadata.
type PageMeta struct {
	HasNextPage bool   `json:"hasNextPage"`
	NextCursor  string `json:"nextCursor"`
}

// ContractPage is a single page of contracts.
type ContractPage struct {
	Data []Contract `json:"data"`
	Meta PageMeta   `json:"meta"`
}

// AssetPage is a single page of assets.
type AssetPage struct {
	Data []Asset  `json:"data"`
	Meta PageMeta `json:"meta"`
}

// ListOptions configures pagination and sorting for list endpoints.
// The zero value is valid and yields the upstream defaults.
type ListOptions struct {
	Limit     int    // 0 = upstream default
	Cursor    string // resume from this cursor
	SortBy    string // server-defined fields
	SortOrder string // "asc" | "desc"
}

// Logger is the minimal logging interface accepted by WithLogger.
type Logger interface {
	Printf(format string, args ...any)
}
