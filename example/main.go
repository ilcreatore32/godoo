// godoo/example/main.go
package main

import (
	"context" // Import for context.Context, WithTimeout, Background
	"errors"  // Import for errors.Is
	"fmt"     // Import for fmt.Println, fmt.Printf
	"log"     // Standard logger for fatal errors in main package
	"os"

	// Import for os.Getenv
	"time" // Import for time.Duration, time.Second, time.Millisecond

	"github.com/ilcreatore32/godoo" // Import the godoo client library
	"go.uber.org/zap"               // Import Zap for logging
)

func main() {
	// Load Odoo connection details from environment variables.
	// This is the recommended practice for sensitive information.
	// DO NOT hardcode credentials in production applications.
	odooURL := os.Getenv("ODOO_URL")
	odooDB := os.Getenv("ODOO_DB")
	odooUsername := os.Getenv("ODOO_USERNAME")
	odooPassword := os.Getenv("ODOO_PASSWORD")
	skipTLSVerify := true // Convert string to boolean

	// Validate that necessary environment variables are set.
	if odooURL == "" || odooDB == "" || odooUsername == "" || odooPassword == "" {
		log.Fatalf("Error: Environment variables ODOO_URL, ODOO_DB, ODOO_USERNAME, and ODOO_PASSWORD must be set.\n" +
			"Please set them before running the example, e.g.:\n" +
			"export ODOO_URL=\"https://your-odoo-instance.com\"\n" +
			"export ODOO_DB=\"your_odoo_database\"\n" +
			"export ODOO_USERNAME=\"your_odoo_user\"\n" +
			"export ODOO_PASSWORD=\"your_odoo_password\"\n" +
			"export ODOO_SKIP_TLS_VERIFY=\"true\" # Only for development/testing",
		)
	}

	// --- Configure the Zap Logger for this example application ---
	// Using zap.NewDevelopment() for human-readable console output during development.
	// In production, you would typically use zap.NewProduction() for structured JSON logs.
	appLogger, err := zap.NewDevelopment()
	if err != nil {
		log.Fatalf("Failed to create application Zap logger: %v", err)
	}
	defer func() {
		_ = appLogger.Sync() // Flushes any buffered log entries before exiting
	}()

	// --- Configure godoo's internal logger based on environment ---
	godooLoggerEnv := godoo.EnvDevelopment

	// Create a new Odoo client instance.
	client, err := godoo.New(
		odooURL,
		odooDB,
		odooUsername,
		odooPassword,
		godoo.WithSkipTLSVerify(skipTLSVerify),
		godoo.WithLoggerEnv(godooLoggerEnv),
		godoo.WithAuthTimeout(3*time.Hour),
		// godoo.WithLogger(appLogger), // Uncomment this line if you want godoo to use appLogger directly
	)
	if err != nil {
		appLogger.Fatal("Failed to initialize Odoo client", zap.Error(err))
	}

	// --- Create a context.Context for all Odoo operations ---
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel() // Important: Always call cancel to release context resources

	// --- Example 1: Basic Search and Read Operations with new types ---
	fmt.Println("\n--- Searching for Companies (res.partner where is_company = true) ---")
	// Using the new Domain type
	companyDomain := godoo.Domain{
		{"is_company", "=", true},
		{"active", "=", true}, // Add an extra condition for demonstration
	}
	companyIDs, err := client.Search(ctx, godoo.ModelResPartner, companyDomain) // Using godoo.ModelResPartner
	if err != nil {
		appLogger.Error("Error searching for companies", zap.Error(err), zap.String("operation", "SearchCompanies"))
		if errors.Is(err, godoo.ErrAuthenticationFailed) {
			fmt.Println(">> Application Error: Authentication failed! Please check Odoo credentials.")
		} else {
			fmt.Printf(">> Application Error: %v\n", err)
		}
	} else {
		fmt.Printf("Found %d company IDs.\n", len(companyIDs))
		if len(companyIDs) > 0 {
			fmt.Println("\n--- Reading details of the first company found ---")
			// Using the new Fields type
			companyFields := godoo.Fields{"name", "email", "phone", "street", "city", "country_id"}
			company, err := client.ReadOne(ctx, godoo.ModelResPartner, companyIDs[0], companyFields) // Using godoo.ModelResPartner and companyFields
			if err != nil {
				appLogger.Error("Error reading company details",
					zap.Error(err),
					zap.Int64("company_id", companyIDs[0]),
					zap.String("operation", "ReadOneCompany"),
				)
				if errors.Is(err, godoo.ErrRecordNotFound) {
					fmt.Printf(">> Application Error: Company with ID %d not found.\n", companyIDs[0])
				} else {
					fmt.Printf(">> Application Error: %v\n", err)
				}
			} else {
				fmt.Printf("Company details: %+v\n", company)
			}
		}
	}

	// --- Example 2: Demonstrate ErrRecordNotFound with new types ---
	fmt.Println("\n--- Demonstrating 'Record Not Found' Error ---")
	// Using the new Domain type for a non-existent company
	nonExistentDomain := godoo.Domain{{"name", "=", "ThisCompanyDoesNotExist12345"}}
	_, err = client.SearchOne(ctx, godoo.ModelResPartner, nonExistentDomain) // Using godoo.ModelResPartner
	if err != nil {
		appLogger.Error("Expected error: SearchOne for non-existent record failed", zap.Error(err))
		if errors.Is(err, godoo.ErrRecordNotFound) {
			fmt.Println(">> Application Status: Successfully caught 'Record Not Found' error.")
		} else {
			fmt.Printf(">> Application Unexpected Error: %v\n", err)
		}
	}

	// --- Example 3: Demonstrate Context Timeout ---
	fmt.Println("\n--- Demonstrating Context Timeout (Simulated) ---")
	ctxTimeout, cancelTimeout := context.WithTimeout(context.Background(), 1*time.Millisecond)
	defer cancelTimeout()

	// Using the new Domain type
	timeoutDomain := godoo.Domain{{"id", ">", 0}}
	_, err = client.Search(ctxTimeout, godoo.ModelResPartner, timeoutDomain) // Using godoo.ModelResPartner
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
			appLogger.Warn("Operation cancelled or timed out due to context", zap.Error(err))
			fmt.Println(">> Application Status: Operation was cancelled/timed out by context as expected.")
		} else {
			appLogger.Error("Unexpected error during timeout demonstration", zap.Error(err))
			fmt.Printf(">> Application Unexpected Error: %v\n", err)
		}
	} else {
		appLogger.Info("Operation completed successfully despite short timeout (unexpected behavior for a short timeout)",
			zap.String("note", "This usually means the RPC call was faster than context cancellation."),
		)
		fmt.Println(">> Application Status: Operation completed successfully (UNEXPECTED for a 1ms timeout).")
	}

	// --- Example 4: Using Complex Domain and ReadWithLimit with Options ---
	fmt.Println("\n--- Using Complex Domain and ReadWithLimit with Options ---")

	// Search for partners that are either companies OR have a specific email or are inactive
	complexDomain := godoo.Domain{
		{"is_company", "=", true},
		{"|"},
		{"email", "ilike", "%example.com"},
		{"active", "=", false},
	}

	// Define specific fields to retrieve
	complexFields := godoo.Fields{"id", "name", "email", "active", "is_company", "city", "country_id"}

	// Perform search with complex domain
	filteredIDs, err := client.Search(ctx, godoo.ModelResPartner, complexDomain)
	if err != nil {
		appLogger.Error("Error searching with complex domain", zap.Error(err))
	} else {
		fmt.Printf("Found %d partners with complex domain.\n", len(filteredIDs))
		if len(filteredIDs) > 0 {
			// Read these partners with ReadWithLimit, using the complex fields and options
			partnersData, err := client.ReadWithLimit(ctx, godoo.ModelResPartner, filteredIDs, complexFields, nil)
			if err != nil {
				appLogger.Error("Error reading with limit and options", zap.Error(err))
			} else {
				fmt.Printf("Read %d partners using ReadWithLimit. First 3:\n", len(partnersData))
				for i, p := range partnersData {
					if i >= 3 {
						break
					}
					fmt.Printf("  ID: %v, Name: %v, Email: %v, Active: %v\n", p["id"], p["name"], p["email"], p["active"])
				}
			}
		}
	}

	// --- Example 5: CreateOne and Update ---
	fmt.Println("\n--- Creating and Updating a Partner ---")

	// Data for the new partner
	newPartnerData := godoo.Data{
		"name":       "Test Partner from Go",
		"email":      "test.go@example.com",
		"is_company": true,
		"active":     true,
		"city":       "Valencia",
		"street":     "Av. Bolívar",
	}

	// Create a new partner
	newPartnerID, err := client.CreateOne(ctx, godoo.ModelResPartner, newPartnerData)
	if err != nil {
		appLogger.Error("Error creating new partner", zap.Error(err))
	} else {
		fmt.Printf("Successfully created new partner with ID: %d\n", newPartnerID)

		// Update the newly created partner
		updateData := godoo.Data{
			"name":  "Updated Test Partner from Go",
			"phone": "+1234567890",
		}
		updated, err := client.Update(ctx, godoo.ModelResPartner, []int64{newPartnerID}, updateData)
		if err != nil {
			appLogger.Error("Error updating partner", zap.Error(err), zap.Int64("partner_id", newPartnerID))
		} else {
			fmt.Printf("Partner with ID %d updated successfully: %t\n", newPartnerID, updated)

			// Read updated data to confirm
			updatedPartner, err := client.ReadOne(ctx, godoo.ModelResPartner, newPartnerID, godoo.Fields{"name", "email", "phone"})
			if err != nil {
				appLogger.Error("Error reading updated partner", zap.Error(err), zap.Int64("partner_id", newPartnerID))
			} else {
				fmt.Printf("Updated partner details: %+v\n", updatedPartner)
			}
		}
	}

	// --- Example 6: Demonstrate Create with multiple records ---
	fmt.Println("\n--- Creating Multiple Partners ---")
	multiCreateData := []godoo.Data{
		{
			"name":       "Multi Partner 1",
			"email":      "multi1@example.com",
			"is_company": false,
		},
		{
			"name":       "Multi Partner 2",
			"email":      "multi2@example.com",
			"is_company": false,
		},
	}

	newMultiIDs, err := client.Create(ctx, godoo.ModelResPartner, multiCreateData)
	if err != nil {
		appLogger.Error("Error creating multiple partners", zap.Error(err))
	} else {
		fmt.Printf("Successfully created multiple partners. IDs: %v\n", newMultiIDs)
		if len(newMultiIDs) > 0 { // Check if any IDs were returned
			fmt.Println("\n--- Deleting Multiple Created Partners ---")
			deleted, err := client.Delete(ctx, godoo.ModelResPartner, newMultiIDs) // Pass newMultiIDs directly
			if err != nil {
				appLogger.Error("Error deleting multiple partners", zap.Error(err))
			} else {
				fmt.Printf("Multiple partners with IDs %v deleted successfully: %t\n", newMultiIDs, deleted)
			}
		}
	}

	// --- Example 7: Clean up the single created partner (if any) ---
	if newPartnerID != 0 { // Check if a partner was successfully created earlier
		fmt.Println("\n--- Cleaning up the single created partner ---")
		deleted, err := client.Delete(ctx, godoo.ModelResPartner, []int64{newPartnerID})
		if err != nil {
			appLogger.Error("Error deleting single created partner", zap.Error(err), zap.Int64("partner_id", newPartnerID))
		} else {
			fmt.Printf("Partner with ID %d deleted successfully: %t\n", newPartnerID, deleted)
		}
	}

	fmt.Println("\nExample execution completed.")
}
