// godoo/methods.go
package godoo

import (
	"context"
	"fmt"

	"go.uber.org/zap"
)

// CallMethod calls a custom method on the specified Odoo model.
func (c *OdooClient) CallMethod(ctx context.Context, model, method string, args ...interface{}) (interface{}, error) { // Add context
	c.logger.Debug("Performing Odoo custom method call",
		zap.String("model", model),
		zap.String("method", method),
		zap.Any("args", args),
		zap.String("op", "CallMethod"),
	)

	uid, rpcClient, err := c.getConnection(ctx) // Pass context
	if err != nil {
		c.logger.Error("Failed to get Odoo connection for custom method call",
			zap.Error(err),
			zap.String("model", model),
			zap.String("method", method),
			zap.String("op", "CallMethod"),
		)
		return nil, err
	}

	params := []interface{}{c.db, uid, c.password, model, method}
	params = append(params, args...) // Append the actual arguments for the Odoo method

	var result interface{}
	// The `execute_kw` method requires a final map for keyword arguments (kwargs).
	// Since CallMethod allows flexible `args...`, we append an empty map if no kwargs are provided.
	// If the last arg is a map, it's assumed to be kwargs for execute_kw.
	// For simplicity here, we assume the provided `args` are directly for the Odoo method,
	// and execute_kw's final kwargs parameter is an empty map unless explicitly passed.
	// More sophisticated handling could check if the last `arg` is `map[string]interface{}`
	// and use it as the kwargs for execute_kw. For now, matching previous behavior.
	err = rpcClient.Call("execute_kw", append(params, map[string]interface{}{}), &result)
	if err != nil {
		c.logger.Error("Failed to execute Odoo custom method",
			zap.Error(err),
			zap.String("model", model),
			zap.String("method", method),
			zap.Any("args", args),
			zap.String("op", "CallMethod"),
		)
		return nil, fmt.Errorf("failed to call method '%s' on model '%s': %w", method, model, err)
	}

	c.logger.Info("Odoo custom method call completed successfully",
		zap.String("model", model),
		zap.String("method", method),
		zap.String("op", "CallMethod"),
	)
	return result, nil
}
