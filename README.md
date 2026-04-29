# tesedi

[![Go](https://github.com/enthus-golang/tesedi/actions/workflows/go.yml/badge.svg)](https://github.com/enthus-golang/tesedi/actions/workflows/go.yml)

Go client library for the [Tesedi](https://www.tesedi.com) partner API.

## Installation

```bash
go get github.com/enthus-golang/tesedi
```

## Usage

```go
package main

import (
	"context"
	"fmt"
	"log"

	"github.com/enthus-golang/tesedi"
)

func main() {
	client := tesedi.New(
		"https://example.tesedi.com/api",
		"https://example.tesedi.com/auth",
		"YOUR_API_KEY",
	)

	contract, err := client.GetContractByNumber(context.Background(), "2137277728")
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("contract %s ends %s\n", contract.ContractNumber, contract.EndDate)

	assets, err := client.GetContractAssets(context.Background(), contract.ContractID, nil)
	if err != nil {
		log.Fatal(err)
	}

	for _, a := range assets.Data {
		fmt.Printf("  %s %s — %s\n", a.ProductSKU, a.SerialNumber, a.ServiceGroup)
	}
}
```

## Options

```go
client := tesedi.New(baseURL, authURL, apiKey,
	tesedi.WithHTTPClient(myHTTPClient),
	tesedi.WithRateLimit(60, 3600),    // requests per minute, per hour (0 = unlimited)
	tesedi.WithRetry(3, 500*time.Millisecond), // max attempts, base backoff
)
```

## License

[Apache License 2.0](./LICENSE)
