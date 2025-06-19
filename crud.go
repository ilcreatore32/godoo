package godoo

import (
	"context"
	"fmt"
	"sync"

	"go.uber.org/zap"
)

// OdooClient represents the Odoo RPC client instance.
// (Assuming this struct exists in client.go or similar)
// It should contain fields like db, uid, password, logger, etc.
// For this crud.go, we'll assume it has a `getConnection` method and `logger`.
//
// type OdooClient struct {
// 	// ... other fields like db, uid, password, logger, rpcClientPool, etc.
// 	db        string
// 	uid       int64
// 	password  string
// 	logger    *zap.Logger
// 	// Mutex to protect connection state (if not using a pool)
// 	// Or a connection pool management
// 	mu        sync.Mutex
// 	rpcClient *xmlrpc.Client // Example, replace with actual RPC client
// }

// executeRPC is a helper method to handle Odoo RPC calls with context timeout/cancellation.
// It abstracts away the goroutine and select logic common to many Odoo client methods.
//
// Parameters:
//   - ctx: The context for the request, enabling cancellation and timeouts.
//   - model: The Odoo model name (as string) to operate on.
//   - method: The Odoo RPC method to call (e.g., "search", "read", "create", "write", "unlink").
//   - args: The arguments for the Odoo RPC method. This should typically be a slice of interfaces.
//   - options: A map of string to interface{} representing keyword arguments for the Odoo method (e.g., {"limit": 10}).
//   - reply: A pointer to the variable where the RPC response will be unmarshalled.
//
// Returns:
//   - error: An error if the RPC call fails, including network issues, Odoo server errors,
//     or context cancellation/timeout.
func (c *OdooClient) executeRPC(ctx context.Context, model, method string, args []interface{}, options map[string]interface{}, reply interface{}) error {
	// Assuming `c.getConnection` manages pooled connections and returns `uid` and `rpcClient`.
	// The `uid` and `rpcClient` are typically short-lived or come from a pool.
	uid, rpcClient, err := c.getConnection(ctx)
	if err != nil {
		c.logger.Error("Failed to get Odoo connection for RPC call",
			zap.Error(err),
			zap.String("model", model),
			zap.String("method", method),
		)
		return err
	}

	// Odoo's execute_kw expects (db, uid, password, model, method, args[], kwargs{})
	callArgs := []interface{}{c.db, uid, c.password, model, method, args}

	// Append options (kwargs) if provided, otherwise an empty map.
	// `execute_kw` always expects a kwargs dictionary, even if empty.
	if len(options) > 0 {
		callArgs = append(callArgs, options)
	} else {
		callArgs = append(callArgs, map[string]interface{}{}) // Pass an empty dict if no options
	}

	callChan := make(chan error, 1)
	go func() {
		// This goroutine executes the blocking RPC call.
		callErr := rpcClient.Call("execute_kw", callArgs, reply)
		callChan <- callErr
	}()

	select {
	case <-ctx.Done():
		c.logger.Error("Odoo RPC call cancelled by context timeout/cancellation",
			zap.Error(ctx.Err()),
			zap.String("model", model),
			zap.String("method", method),
		)
		return ctx.Err() // Return the context's error
	case err = <-callChan:
		// The RPC call completed (successfully or with an error).
		// `err` now holds the result of `rpcClient.Call`.
		if err != nil {
			c.logger.Error("Failed to execute Odoo RPC call",
				zap.Error(err),
				zap.String("model", model),
				zap.String("method", method),
			)
			// Parse the error to a more specific OdooRPCError if possible.
			return parseOdooRPCError(fmt.Errorf("failed to call Odoo method '%s' on model '%s': %w", method, model, err))
		}
	}
	return nil
}

// --- CRUD Operations ---

// Search performs a search operation on the specified Odoo model.
// It returns a slice of integer IDs (int64) that match the given domain.
//
// Parameters:
//   - ctx: The context for the request, enabling cancellation and timeouts.
//   - model: The Odoo model name (e.g., ModelResPartner, ModelProductTemplate).
//   - domain: A Domain type representing the Odoo domain filter.
//     Example: `godoo.Domain{{"name", "=", "John Doe"}, {"active", "=", true}}`
//     For complex logical operations like OR/AND, proper nesting is required:
//     `godoo.Domain{"&", {"is_company", "=", true}, {"|", {"email", "ilike", "%example.com"}, {"active", "=", false}}}`
//   - options: Optional pointer to an Options struct to control search parameters like limit, offset, order, and context.
//
// Returns:
//   - []int64: A slice of IDs of the records that match the search criteria.
//   - error: An error if the operation fails, including network issues, Odoo RPC errors,
//     or context cancellation/timeout. Returns `ErrRecordNotFound` if no records match the domain (though Odoo search usually returns empty list, not error).
func (c *OdooClient) Search(ctx context.Context, model Model, domain Domain, options ...*Options) ([]int64, error) {
	c.logger.Debug("Performing Odoo search",
		zap.String("model", string(model)),
		zap.Any("domain", domain), // Log the Domain as is for debugging structure
		zap.String("op", "Search"),
	)

	var ids []int64
	// `domain.ToRPC()` correctly converts godoo.Domain (which is []interface{}) to []interface{}.
	// `c.parseOptions(options...)` handles the optional Options struct.
	err := c.executeRPC(ctx, string(model), "search", []interface{}{domain.ToRPC()}, c.parseOptions(options...), &ids)
	if err != nil {
		return nil, err
	}

	c.logger.Info("Odoo search completed",
		zap.String("model", string(model)),
		zap.Int("results", len(ids)),
		zap.String("op", "Search"),
	)
	return ids, nil
}

// SearchOne performs a search operation on the specified Odoo model,
// but limits the result to at most one record.
// This is useful when you expect a unique record based on the domain.
//
// Parameters:
//   - ctx: The context for the request.
//   - model: The Odoo model name.
//   - domain: The Domain type representing the Odoo domain filter.
//   - options: Optional pointer to an Options struct to include additional context or override default behavior.
//     Note: The `Limit` option will be internally set to 1 for this method.
//
// Returns:
//   - int64: The ID of the single record found.
//   - error: An error if the operation fails, or `ErrRecordNotFound` if no record is found.
//     If multiple records are found (which should not happen with limit 1, but as a safeguard),
//     it logs a warning and returns the first ID.
func (c *OdooClient) SearchOne(ctx context.Context, model Model, domain Domain, options ...*Options) (int64, error) {
	c.logger.Debug("Performing Odoo searchOne",
		zap.String("model", string(model)),
		zap.Any("domain", domain),
		zap.String("op", "SearchOne"),
	)

	// Prepare options, ensuring Limit is set to 1.
	searchOptions := &Options{Limit: 1}
	if len(options) > 0 && options[0] != nil {
		// Merge provided options, but ensure limit is 1.
		mergedOptions := options[0].ToRPC()
		mergedOptions["limit"] = 1
		searchOptions = &Options{ // Recreate Options struct to pass to parseOptions
			Limit:   1,
			Offset:  options[0].Offset,
			Order:   options[0].Order,
			Context: options[0].Context,
			Extra:   options[0].Extra,
		}
	}

	var ids []int64
	err := c.executeRPC(ctx, string(model), "search", []interface{}{domain.ToRPC()}, searchOptions.ToRPC(), &ids)
	if err != nil {
		return 0, err
	}

	if len(ids) == 0 {
		c.logger.Info("No records found for Odoo searchOne",
			zap.String("model", string(model)),
			zap.Any("domain", domain),
			zap.String("op", "SearchOne"),
		)
		return 0, fmt.Errorf("%w: for model '%s' with domain %v", ErrRecordNotFound, string(model), domain.ToRPC())
	}
	if len(ids) > 1 {
		c.logger.Warn("SearchOne found more than one record despite limit=1, returning the first",
			zap.String("model", string(model)),
			zap.Any("domain", domain),
			zap.Int("found_count", len(ids)),
		)
	}

	c.logger.Info("Odoo searchOne completed",
		zap.String("model", string(model)),
		zap.Int64("result_id", ids[0]),
		zap.String("op", "SearchOne"),
	)
	return ids[0], nil
}

// Read performs a read operation on the specified Odoo model, retrieving records by their IDs.
// It fetches specific fields for the given records.
//
// Parameters:
//   - ctx: The context for the request.
//   - model: The Odoo model name.
//   - ids: A slice of int64 representing the IDs of the records to read.
//   - fields: A Fields type representing the names of the fields to retrieve.
//     If `fields` is empty or nil, Odoo will typically return a default set of fields.
//   - options: Optional pointer to an Options struct to include additional context or override default behavior.
//
// Returns:
//   - []map[string]interface{}: A slice of maps, where each map represents a record
//     and contains field-value pairs.
//   - error: An error if the operation fails, or if parsing the response fails.
func (c *OdooClient) Read(ctx context.Context, model Model, ids []int64, fields Fields, options ...*Options) ([]map[string]interface{}, error) {
	c.logger.Debug("Performing Odoo read",
		zap.String("model", string(model)),
		zap.Any("ids", ids),
		zap.Any("fields", fields),
		zap.String("op", "Read"),
	)

	if len(ids) == 0 {
		c.logger.Info("No IDs provided for Odoo read, returning empty slice",
			zap.String("model", string(model)),
			zap.String("op", "Read"),
		)
		return []map[string]interface{}{}, nil
	}

	var records []map[string]interface{}
	// `fields.ToRPC()` correctly converts godoo.Fields to []string.
	// `c.parseOptions(options...)` handles the optional Options struct.
	err := c.executeRPC(ctx, string(model), "read", []interface{}{ids, fields.ToRPC()}, c.parseOptions(options...), &records)
	if err != nil {
		return nil, err
	}

	c.logger.Info("Odoo read completed",
		zap.String("model", string(model)),
		zap.Int("records_count", len(records)),
		zap.String("op", "Read"),
	)
	return records, nil
}

// ReadOne performs a read operation for a single record on the specified Odoo model.
// It retrieves specific fields for the given record ID.
//
// Parameters:
//   - ctx: The context for the request.
//   - model: The Odoo model name.
//   - id: The int64 ID of the single record to read.
//   - fields: A Fields type representing the names of the fields to retrieve.
//     If `fields` is empty or nil, Odoo will return a default set of fields.
//   - options: Optional pointer to an Options struct to include additional context or override default behavior.
//
// Returns:
//   - map[string]interface{}: A map representing the single record, containing field-value pairs.
//   - error: An error if the operation fails, or `ErrRecordNotFound` if no record is found for the given ID.
func (c *OdooClient) ReadOne(ctx context.Context, model Model, id int64, fields Fields, options ...*Options) (map[string]interface{}, error) {
	c.logger.Debug("Performing Odoo readOne",
		zap.String("model", string(model)),
		zap.Int64("id", id),
		zap.Any("fields", fields),
		zap.String("op", "ReadOne"),
	)

	// Call the more general Read method.
	records, err := c.Read(ctx, model, []int64{id}, fields, options...)
	if err != nil {
		return nil, err
	}

	if len(records) == 0 {
		c.logger.Info("No record found for Odoo readOne",
			zap.String("model", string(model)),
			zap.Int64("id", id),
			zap.String("op", "ReadOne"),
		)
		return nil, fmt.Errorf("%w: for model '%s' with ID %v", ErrRecordNotFound, string(model), id)
	}
	// If more than one record is returned (highly unlikely for a single ID read),
	// we still return the first one as expected by ReadOne's contract.
	c.logger.Info("Odoo readOne completed",
		zap.String("model", string(model)),
		zap.Int64("record_id", id),
		zap.String("op", "ReadOne"),
	)
	return records[0], nil
}

// ReadWithLimit performs a read operation on the specified Odoo model with IDs, fields, and options.
// This is useful for fetching a subset of records when dealing with a large set of IDs,
// and applying specific Odoo context or ordering. Note that this method does not perform an
// internal 'search' prior to 'read'; it expects the IDs to be provided directly.
// Use `Search` followed by `Read` if filtering is needed.
//
// Parameters:
//   - ctx: The context for the request.
//   - model: The Odoo model name.
//   - ids: A slice of int64 representing the IDs of the records to read.
//   - fields: A Fields type representing the names of the fields to retrieve.
//   - options: A pointer to an Options struct, allowing specification of limit, offset, order, and context.
//     This argument is not optional and should always be provided, even if empty, to configure the read behavior.
//
// Returns:
//   - []map[string]interface{}: A slice of maps, where each map represents a record.
//   - error: An error if the operation fails.
func (c *OdooClient) ReadWithLimit(ctx context.Context, model Model, ids []int64, fields Fields, options *Options) ([]map[string]interface{}, error) {
	c.logger.Debug("Performing Odoo readWithLimit",
		zap.String("model", string(model)),
		zap.Any("ids", ids),
		zap.Any("fields", fields),
		zap.Any("options", options),
		zap.String("op", "ReadWithLimit"),
	)

	if len(ids) == 0 {
		c.logger.Info("No IDs provided for Odoo readWithLimit, returning empty slice",
			zap.String("model", string(model)),
			zap.String("op", "ReadWithLimit"),
		)
		return []map[string]interface{}{}, nil
	}
	if options == nil {
		options = &Options{} // Ensure options is not nil for ToRPC call
	}

	var records []map[string]interface{}
	// `fields.ToRPC()` correctly converts godoo.Fields to []string.
	// `options.ToRPC()` converts godoo.Options to map[string]interface{}.
	err := c.executeRPC(ctx, string(model), "read", []interface{}{ids, fields.ToRPC()}, options.ToRPC(), &records)
	if err != nil {
		return nil, err
	}

	c.logger.Info("Odoo readWithLimit completed",
		zap.String("model", string(model)),
		zap.Int("records_count", len(records)),
		zap.String("op", "ReadWithLimit"),
	)
	return records, nil
}

// CreateOne creates a single new record in the specified Odoo model.
// It returns the ID (int64) of the newly created record.
//
// Parameters:
//   - ctx: The context for the request.
//   - model: The Odoo model name where the record will be created.
//   - data: A Data type representing the field-value pairs for the new record.
//     Example: `godoo.Data{"name": "Single Product", "price": 25.0}`.
//   - options: Optional pointer to an Options struct to include additional context.
//
// Returns:
//   - int64: The ID of the newly created record.
//   - error: An error if the creation fails, or if the response type is unexpected.
func (c *OdooClient) CreateOne(ctx context.Context, model Model, data Data, options ...*Options) (int64, error) {
	c.logger.Debug("Performing Odoo createOne",
		zap.String("model", string(model)),
		zap.Any("data", data),
		zap.String("op", "CreateOne"),
	)

	var newIDs []int64 // Changed to expect a slice for the reply
	// Odoo's 'create' method expects a list of dictionaries for the data argument.
	// Even for a single record, it's `[{"field1": "value1", "field2": "value2"}]`.
	// So, we wrap `data.ToRPC()` in a slice.
	err := c.executeRPC(ctx, string(model), "create", []interface{}{[]map[string]interface{}{data.ToRPC()}}, c.parseOptions(options...), &newIDs)
	if err != nil {
		return 0, err
	}

	if len(newIDs) == 0 {
		return 0, fmt.Errorf("%w: Odoo did not return an ID for single record creation", ErrInvalidResponse)
	}
	if len(newIDs) > 1 {
		c.logger.Warn("CreateOne returned multiple IDs, returning the first one",
			zap.String("model", string(model)),
			zap.Any("ids", newIDs),
		)
	}

	c.logger.Info("Odoo createOne completed",
		zap.String("model", string(model)),
		zap.Int64("new_id", newIDs[0]),
		zap.String("op", "CreateOne"),
	)
	return newIDs[0], nil
}

// Create creates one or more new records in the specified Odoo model.
//
// Parameters:
//   - ctx: The context for the request.
//   - model: The Odoo model name where the records will be created.
//   - data: A slice of Data types, where each Data map represents the field-value pairs
//     for a new record.
//     Example: `[]godoo.Data{{"name": "Prod 1"}, {"name": "Prod 2"}}`.
//   - options: Optional pointer to an Options struct to include additional context.
//
// Returns:
//   - []int64: A slice of IDs of the newly created records. Returns an empty slice if no records are created.
//   - error: An error if the creation fails, or if the response type is unexpected.
//     Note: Odoo's RPC usually returns `[]int64` for multiple creations.
func (c *OdooClient) Create(ctx context.Context, model Model, data []Data, options ...*Options) ([]int64, error) {
	c.logger.Debug("Performing Odoo create (multiple records)",
		zap.String("model", string(model)),
		zap.Int("data_entries", len(data)),
		zap.String("op", "Create"),
	)

	if len(data) == 0 {
		c.logger.Info("No data provided for Odoo create, returning empty slice",
			zap.String("model", string(model)),
			zap.String("op", "Create"),
		)
		return []int64{}, nil
	}

	// Convert []godoo.Data to []map[string]interface{} as expected by Odoo's create method.
	dataToRPC := make([]map[string]interface{}, len(data))
	for i, d := range data {
		dataToRPC[i] = d.ToRPC()
	}

	var newIDs []int64 // Expecting a slice of int64 IDs for multiple creation
	err := c.executeRPC(ctx, string(model), "create", []interface{}{dataToRPC}, c.parseOptions(options...), &newIDs)
	if err != nil {
		return nil, err
	}

	c.logger.Info("Odoo create (multiple records) completed",
		zap.String("model", string(model)),
		zap.Any("new_ids", newIDs),
		zap.String("op", "Create"),
	)
	return newIDs, nil
}

// Update updates existing records in the specified Odoo model.
//
// Parameters:
//   - ctx: The context for the request.
//   - model: The Odoo model name where records will be updated.
//   - ids: A slice of int64 representing the IDs of the records to update.
//   - data: A Data type where keys are field names (string) and values are the new
//     data for those fields. Only the fields specified in `data` will be updated.
//     Example: `godoo.Data{"name": "Updated Product Name", "price": 12.0}`.
//   - options: Optional pointer to an Options struct to include additional context.
//
// Returns:
//   - bool: `true` if the update operation was successful, `false` otherwise.
//   - error: An error if the update fails, or if the response type is unexpected.
func (c *OdooClient) Update(ctx context.Context, model Model, ids []int64, data Data, options ...*Options) (bool, error) {
	c.logger.Debug("Performing Odoo update",
		zap.String("model", string(model)),
		zap.Any("ids", ids),
		zap.Any("data", data), // Log the Data as is for debugging
		zap.String("op", "Update"),
	)

	if len(ids) == 0 {
		return false, fmt.Errorf("godoo: no record IDs provided for update")
	}

	var success bool
	// Odoo's 'write' method expects a list of IDs and a dictionary of data.
	err := c.executeRPC(ctx, string(model), "write", []interface{}{ids, data.ToRPC()}, c.parseOptions(options...), &success)
	if err != nil {
		return false, err
	}

	c.logger.Info("Odoo update completed",
		zap.String("model", string(model)),
		zap.Any("ids", ids),
		zap.Bool("success", success),
		zap.String("op", "Update"),
	)
	return success, nil
}

// UpdateMultiple updates multiple existing records in the specified Odoo model,
// allowing different data to be applied to each record.
//
// This function iterates through the provided map of IDs and their respective data,
// making an individual Odoo RPC call for each record concurrently using goroutines.
// This can improve performance for a large number of independent record updates.
//
// Parameters:
//   - ctx: The context for the request, enabling cancellation and timeouts for each individual update.
//   - model: The Odoo model name where records will be updated.
//   - idDataMap: A map where keys are the int64 IDs of the records to update,
//     and values are Data maps containing the specific field-value
//     pairs for each corresponding record.
//     Example:
//     map[int64]godoo.Data{
//     1: {"name": "Updated Item 1", "active": true},
//     2: {"description": "New description for item 2"},
//     }
//
// Returns:
//   - map[int64]error: A map indicating the success or failure for each ID.
//     If an ID was updated successfully, its value in the map will be nil.
//     If an error occurred for a specific ID, the error will be present.
//     This map will be empty if idDataMap is empty or nil.
//   - error: An error if there's a fundamental issue before starting updates
//     (e.g., connection failure), or if the main context is cancelled.
//     Individual record errors are captured in the returned map.
func (c *OdooClient) UpdateMultiple(ctx context.Context, model Model, idDataMap map[int64]Data, options ...*Options) (map[int64]error, error) {
	c.logger.Debug("Performing Odoo updateMultiple",
		zap.String("model", string(model)),
		zap.Int("records_to_update", len(idDataMap)),
		zap.String("op", "UpdateMultiple"),
	)

	if len(idDataMap) == 0 {
		c.logger.Info("No records to update in Odoo updateMultiple, returning empty results",
			zap.String("model", string(model)),
			zap.String("op", "UpdateMultiple"),
		)
		return map[int64]error{}, nil
	}

	resultsChan := make(chan struct {
		ID  int64
		Err error
	}, len(idDataMap))

	var wg sync.WaitGroup
	parsedOptions := c.parseOptions(options...) // Parse options once for all concurrent calls

	for id, data := range idDataMap {
		wg.Add(1)
		go func(recordID int64, recordData Data) { // Changed to Data type
			defer wg.Done()
			var success bool
			// `recordData.ToRPC()` converts godoo.Data to map[string]interface{}.
			err := c.executeRPC(ctx, string(model), "write", []interface{}{[]int64{recordID}, recordData.ToRPC()}, parsedOptions, &success)
			resultsChan <- struct {
				ID  int64
				Err error
			}{ID: recordID, Err: err}
		}(id, data)
	}

	wg.Wait()
	close(resultsChan)

	failedUpdates := make(map[int64]error)
	for res := range resultsChan {
		if res.Err != nil {
			failedUpdates[res.ID] = res.Err
			c.logger.Error("Failed to update single record in Odoo updateMultiple",
				zap.Int64("record_id", res.ID),
				zap.String("model", string(model)),
				zap.Error(res.Err),
				zap.String("op", "UpdateMultiple"),
			)
		}
	}

	if ctx.Err() != nil {
		return nil, ctx.Err() // Return context error if the main context was cancelled
	}
	return failedUpdates, nil
}

// Delete deletes records from the specified Odoo model.
//
// Parameters:
//   - ctx: The context for the request.
//   - model: The Odoo model name from which records will be deleted.
//   - ids: A slice of int64 representing the IDs of the records to delete.
//   - options: Optional pointer to an Options struct to include additional context.
//
// Returns:
//   - bool: `true` if the deletion operation was successful, `false` otherwise.
//   - error: An error if the deletion fails, or if the response type is unexpected.
func (c *OdooClient) Delete(ctx context.Context, model Model, ids []int64, options ...*Options) (bool, error) {
	c.logger.Debug("Performing Odoo delete",
		zap.String("model", string(model)),
		zap.Any("ids", ids),
		zap.String("op", "Delete"),
	)

	if len(ids) == 0 {
		return false, fmt.Errorf("godoo: no record IDs provided for deletion")
	}

	var success bool
	// Odoo's 'unlink' method expects a list of IDs.
	err := c.executeRPC(ctx, string(model), "unlink", []interface{}{ids}, c.parseOptions(options...), &success)
	if err != nil {
		return false, err
	}

	c.logger.Info("Odoo delete completed",
		zap.String("model", string(model)),
		zap.Any("ids", ids),
		zap.Bool("success", success),
		zap.String("op", "Delete"),
	)
	return success, nil
}

// CallOdoo executes a custom Odoo RPC method call using 'execute_kw'.
// This function provides maximum flexibility for calling any Odoo model method
// with custom arguments and options, including the Odoo context.
//
// Parameters:
//   - ctx: The Go context for the request, enabling cancellation and timeouts.
//   - model: The Odoo model name (e.g., ModelResPartner, ModelProductTemplate).
//   - method: The name of the Odoo model method to call (e.g., "search", "read", "create", "write", "unlink", or a custom method like "my_custom_action").
//   - args: A slice of interfaces representing the positional arguments for the Odoo method.
//     This corresponds to the third argument of Odoo's `execute_kw` call, which is a list of arguments for the method being called.
//     Example for `read`: `[]interface{}{[]int64{1, 2, 3}, []string{"name", "display_name"}}`
//     When using `Domain`, `Fields`, or `Data` types, remember to call their `.ToRPC()` method or cast to their underlying types.
//   - options: A map of string to interface{} representing keyword arguments for the Odoo method.
//     This corresponds to the fourth argument of Odoo's `execute_kw` call, which is a dictionary of options.
//     This is where Odoo's `context` (e.g., `{"context": {"lang": "es_ES"}}`), `limit`, `offset`, `order`, etc., are passed.
//     If using `godoo.Options`, call `myOptions.ToRPC()`.
//
// Returns:
//   - interface{}: The raw result from the Odoo RPC call. The caller is responsible for type asserting this value
//     to the expected Go type (e.g., `int64`, `[]int64`, `map[string]interface{}`, `[]map[string]interface{}`, `bool`, etc.).
//   - error: An error if the operation fails due to connection issues, Odoo RPC errors, or context cancellation/timeout.
func (c *OdooClient) CallOdoo(ctx context.Context, model Model, method string, args []interface{}, options map[string]interface{}) (interface{}, error) {
	c.logger.Debug("Performing custom Odoo RPC call",
		zap.String("model", string(model)),
		zap.String("method", method),
		zap.Any("args", args),
		zap.Any("options", options),
		zap.String("op", "CallOdoo"),
	)

	uid, rpcClient, err := c.getConnection(ctx)
	if err != nil {
		c.logger.Error("Failed to get Odoo connection for custom RPC call",
			zap.Error(err),
			zap.String("model", string(model)),
			zap.String("method", method),
			zap.String("op", "CallOdoo"),
		)
		return nil, err
	}

	var result interface{} // The response can be of any type

	// Construct the arguments for the rpcClient.Call("execute_kw", ...)
	// This mirrors the structure: (db, uid, password, model, method, args[], options{})
	rpcCallArgs := []interface{}{c.db, uid, c.password, string(model), method, args}

	// Append options (kwargs) if provided, otherwise an empty map.
	// `execute_kw` always expects a kwargs dictionary, even if empty.
	if len(options) > 0 {
		rpcCallArgs = append(rpcCallArgs, options)
	} else {
		rpcCallArgs = append(rpcCallArgs, map[string]interface{}{})
	}

	callChan := make(chan error, 1)
	go func() {
		// Execute the RPC call. The result will be unmarshalled into 'result'
		callErr := rpcClient.Call("execute_kw", rpcCallArgs, &result)
		callChan <- callErr
	}()

	select {
	case <-ctx.Done():
		c.logger.Error("Custom Odoo RPC call cancelled by context timeout/cancellation",
			zap.Error(ctx.Err()),
			zap.String("model", string(model)),
			zap.String("method", method),
			zap.String("op", "CallOdoo"),
		)
		return nil, ctx.Err()
	case err = <-callChan:
		// The RPC call completed (successfully or with an error)
		if err != nil {
			c.logger.Error("Failed to execute custom Odoo RPC call",
				zap.Error(err),
				zap.String("model", string(model)),
				zap.String("method", method),
				zap.String("op", "CallOdoo"),
			)
			return nil, parseOdooRPCError(fmt.Errorf("failed to call Odoo method '%s' on model '%s': %w", method, string(model), err))
		}
	}

	c.logger.Info("Custom Odoo RPC call completed",
		zap.String("model", string(model)),
		zap.String("method", method),
		zap.Any("result", result), // Log the raw result (be careful with large results)
		zap.String("op", "CallOdoo"),
	)
	return result, nil
}
