# `godoo` - Odoo XML-RPC Client for Go

`godoo` is a Go library providing a simple and flexible client for interacting with Odoo instances via its XML-RPC API (Odoo 12+). It offers a clean and idiomatic Go interface for common Odoo operations such as search, read, create, update, delete, and custom method calls.

-----

## Table of Contents

- [Features](https://www.google.com/search?q=%23features)
- [Installation](https://www.google.com/search?q=%23installation)
- [Usage](https://www.google.com/search?q=%23usage)
  - [Quick Start](https://www.google.com/search?q=%23quick-start)
  - [Handling Errors](https://www.google.com/search?q=%23handling-errors)
  - [Custom Method Calls](https://www.google.com/search?q=%23custom-method-calls)
- [Configuration Options](https://www.google.com/search?q=%23configuration-options)
- [Compatibility](https://www.google.com/search?q=%23compatibility)
- [Contributing](https://www.google.com/search?q=%23contributing)
- [License](https://www.google.com/search?q=%23license)

-----

## Features

- **XML-RPC Communication:** Connects to Odoo's `common` and `object` endpoints.
- **Authentication Management:** Handles Odoo user authentication and session validity.
- **CRUD Operations:**
  - `Search`: Search records by domain.
  - `SearchOne`: Search for a single record.
  - `Read`: Read multiple records by ID and fields.
  - `ReadOne`: Read a single record by ID and fields.
  - `ReadWithLimit`: Read records with a specified limit.
  - `Create`: Create new records.
  - `Update`: Update existing records.
  - `Delete`: Delete records.
- **Custom Method Calls:** `CallMethod` for invoking any custom Odoo method.
- **Context Support (`context.Context`):** All operations accept `context.Context` for cancellation and timeouts, enabling robust and controllable network interactions.
- **Flexible Logging with Zap:**
  - Uses `go.uber.org/zap` for high-performance, structured logging.
  - Provides a default production-ready logger (JSON output).
  - Allows configuration for development (human-readable) or production logging via functional options.
  - Supports injecting a completely custom `*zap.Logger` instance.
- **Configurable Options:** Utilize functional options (`godoo.With...`) for easy setup of authentication timeouts, TLS verification skipping (for development/testing), and custom HTTP clients.

-----

## Installation

To install `godoo`, use `go get`:

```bash
go get github.com/ilcreatore32/godoo
```

To fetch a specific version (recommended for production applications), use:

```bash
go get github.com/ilcreatore32/godoo@vX.Y.Z
```

(Replace `vX.Y.Z` with a [release tag](https://www.google.com/search?q=https://github.com/ilcreatore32/godoo/releases) like `v0.1.0`).

-----

## Usage

### Quick Start

Here's a basic example of how to initialize `godoo` and perform a simple search operation:

```go
package main

import (
 "context"
 "fmt"
 "log"
 "os"
 "time"

 "github.com/ilcreatore32/godoo"
 "go.uber.org/zap"
)

func main() {
 // 1. Load Odoo connection details from environment variables.
 //    DO NOT hardcode credentials in production.
 odooURL := os.Getenv("ODOO_URL")
 odooDB := os.Getenv("ODOO_DB")
 odooUsername := os.Getenv("ODOO_USERNAME")
 odooPassword := os.Getenv("ODOO_PASSWORD")
 
 if odooURL == "" || odooDB == "" || odooUsername == "" || odooPassword == "" {
  log.Fatalf("Error: Odoo environment variables are not set. See README for details.")
 }

 // 2. Configure your application's logger (optional, but good practice)
 appLogger, _ := zap.NewDevelopment()
 defer func() { _ = appLogger.Sync() }()

 // 3. Initialize the godoo client
 client, err := godoo.New(
  odooURL,
  odooDB,
  odooUsername,
  odooPassword,
  godoo.WithLoggerEnv(godoo.EnvDevelopment), // Use development logger for godoo
  godoo.WithAuthTimeout(3*time.Hour),
  // godoo.WithSkipTLSVerify(true), // DANGER: Do not use in production!
 )
 if err != nil {
  appLogger.Fatal("Failed to initialize Odoo client", zap.Error(err))
 }

 // 4. Create a context with a timeout for your operations
 ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
 defer cancel()

 // 5. Perform an Odoo operation: Search for companies
 fmt.Println("\n--- Searching for Companies ---")
 companyIDs, err := client.Search(ctx, "res.partner", [][]interface{}{{"is_company", "=", true}})
 if err != nil {
  appLogger.Error("Error searching for companies", zap.Error(err))
  return
 }
 fmt.Printf("Found %d company IDs.\n", len(companyIDs))

 if len(companyIDs) > 0 {
  // 6. Read details of the first company found
  fmt.Println("\n--- Reading details of the first company ---")
  company, err := client.ReadOne(ctx, "res.partner", companyIDs[0], []string{"name", "email", "phone"})
  if err != nil {
   appLogger.Error("Error reading company details", zap.Error(err))
   return
  }
  fmt.Printf("Company details: %+v\n", company)
 }

 fmt.Println("\nExample completed.")
}
```

For more detailed examples, including demonstration of context timeouts and different error types, please refer to the `example/` directory:

- [Full Example with Zap Logging (`example/main.go`)](https://www.google.com/search?q=example/main.go)
- [Simple Example without Zap Logging (`example/simple_example/main.go`)](https://www.google.com/search?q=example/simple_example/main.go)

### Handling Errors

`godoo` returns standard Go `error` types. For specific Odoo-related errors, `godoo` provides custom error types that can be checked using `errors.Is`:

- **`godoo.ErrAuthenticationFailed`**: Returned when Odoo authentication fails (e.g., wrong username/password).
- **`godoo.ErrRecordNotFound`**: Returned by `SearchOne` or `ReadOne` if no record matches the criteria.
- **`godoo.ErrOdooRPC`**: A general wrapper for errors returned directly by the Odoo XML-RPC server (e.g., "Access Denied"). You can check if an error is an Odoo RPC error using `errors.Is(err, godoo.ErrOdooRPC)`. The underlying Odoo error message will be embedded.

**Example of Error Checking:**

```go
import (
    "errors"
    "github.com/ilcreatore32/godoo"
)

// ... inside a function ...
_, err := client.SearchOne(ctx, "res.partner", [][]interface{}{{"name", "=", "NonExistentCompany"}})
if err != nil {
    if errors.Is(err, godoo.ErrRecordNotFound) {
        fmt.Println("Record not found as expected.")
    } else if errors.Is(err, godoo.ErrAuthenticationFailed) {
        fmt.Println("Authentication failed! Check credentials.")
    } else {
        fmt.Printf("An unexpected error occurred: %v\n", err)
    }
}
```

### Custom Method Calls

You can call any custom Odoo method using `client.CallMethod`. This is useful for invoking server actions, wizard methods, or any method not covered by the standard CRUD functions.

```go
package main

import (
 "context"
 "fmt"
 "log"
 "os"
 "time"

 "github.com/ilcreatore32/godoo"
 "go.uber.org/zap"
)

func main() {
 // ... (client initialization as above) ...

 odooURL := os.Getenv("ODOO_URL")
 odooDB := os.Getenv("ODOO_DB")
 odooUsername := os.Getenv("ODOO_USERNAME")
 odooPassword := os.Getenv("ODOO_PASSWORD")
 
 if odooURL == "" || odooDB == "" || odooUsername == "" || odooPassword == "" {
  log.Fatalf("Error: Odoo environment variables are not set. See README for details.")
 }

 appLogger, _ := zap.NewDevelopment()
 defer func() { _ = appLogger.Sync() }()

 client, err := godoo.New(
  odooURL,
  odooDB,
  odooUsername,
  odooPassword,
  godoo.WithLoggerEnv(godoo.EnvDevelopment),
 )
 if err != nil {
  appLogger.Fatal("Failed to initialize Odoo client", zap.Error(err))
 }

 ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
 defer cancel()

 fmt.Println("\n--- Calling a Custom Odoo Method (e.g., 'check_access_rights') ---")
 var result bool // The type of `result` depends on what your Odoo method returns
 
 // Example: Call `check_access_rights` on `res.partner` model
 // Parameters: model, method_name, args (list), kwargs (map)
 err = client.CallMethod(ctx, "res.partner", "check_access_rights", []interface{}{"read", false}, map[string]interface{}{}, &result)
 if err != nil {
  appLogger.Error("Error calling custom method", zap.Error(err))
  return
 }
 fmt.Printf("Result of 'check_access_rights' for 'res.partner' (read): %t\n", result)

 fmt.Println("\nCustom method call example completed.")
}
```

-----

## Configuration Options

When creating a new `godoo.OdooClient` using `godoo.New()`, you can provide several **functional options**:

- **`godoo.WithAuthTimeout(d time.Duration)`**: Sets the duration after which the Odoo authentication session is considered expired and a re-authentication attempt will be made.

- **`godoo.WithSkipTLSVerify(skip bool)`**: **WARNING: DO NOT USE IN PRODUCTION.** If `true`, TLS certificate verification will be skipped. Useful for development or testing with self-signed certificates.

- **`godoo.WithHTTPClient(httpClient *http.Client)`**: Allows you to provide a custom `*http.Client` instance. This is useful for advanced scenarios like custom `Transport` implementations, proxy configurations, or fine-grained control over HTTP timeouts.

- **`godoo.WithLogger(logger *zap.Logger)`**: Injects a pre-configured `*zap.Logger` instance directly into the `OdooClient`. This overrides any settings from `WithLoggerEnv`.

- **`godoo.WithLoggerEnv(env godoo.LoggerEnv)`**: Configures `godoo`'s internal Zap logger based on a predefined environment type:

  - `godoo.EnvDevelopment`: Configures a human-readable logger (similar to `zap.NewDevelopment()`) suitable for console output during development. Disables `caller` info and most stacktraces for cleaner logs.
  - `godoo.EnvProduction`: Configures a high-performance, structured (JSON) logger (similar to `zap.NewProduction()`) suitable for production environments and log aggregation systems. Includes `caller` info and stacktraces for errors.

    If `WithLoggerEnv` is not provided, the `OdooClient` defaults to `godoo.EnvProduction` for its internal logging.

-----

## Compatibility

`godoo` is designed to work with **Odoo 12 and newer** versions, utilizing their XML-RPC API. Compatibility with older versions is not guaranteed.

-----

## Contributing

Contributions are welcome\! If you find a bug, have a feature request, or want to contribute code, please feel free to:

1. Open an [issue](https://www.google.com/search?q=https://github.com/ilcreatore32/godoo/issues).
2. Submit a [pull request](https://www.google.com/search?q=https://github.com/ilcreatore32/godoo/pulls).

-----

## License

`godoo` is licensed under the [MIT License](https://www.google.com/search?q=LICENSE).

-----
