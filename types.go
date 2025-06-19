package godoo

// types.go

// Model represents an Odoo model name.
// This type provides compile-time safety and enables autocompletion
// in IDEs when using predefined model constants.
type Model string

// Predefined constants for commonly used Odoo models.
// Expand this list as needed for your application.
const (
	// Product & Inventory Models
	ModelProductProduct  Model = "product.product"  // Product Variants
	ModelProductTemplate Model = "product.template" // Product Templates
	ModelProductCategory Model = "product.category" // Internal Product Categories
	ModelStockPicking    Model = "stock.picking"    // Delivery Orders, Receipts, Internal Transfers
	ModelStockMove       Model = "stock.move"       // Individual Stock Movements
	ModelStockQuant      Model = "stock.quant"      // Inventory Quantities (on hand by location, lot, etc.)
	ModelStockLocation   Model = "stock.location"   // Warehouse Locations

	// Sales & CRM Models
	ModelSaleOrder     Model = "sale.order"      // Sales Orders
	ModelSaleOrderLine Model = "sale.order.line" // Lines within a Sales Order
	ModelCrmLead       Model = "crm.lead"        // CRM Leads/Opportunities
	ModelCrmStage      Model = "crm.stage"       // Stages for CRM Leads/Opportunities
	ModelResPartner    Model = "res.partner"     // Contacts (Customers, Vendors, Addresses)

	// Accounting & Finance Models
	ModelAccountMove     Model = "account.move"      // Journal Entries, Invoices, Vendor Bills, Refunds
	ModelAccountMoveLine Model = "account.move.line" // Lines within an Account Move
	ModelAccountPayment  Model = "account.payment"   // Customer Payments, Vendor Payments
	ModelAccountJournal  Model = "account.journal"   // Accounting Journals (e.g., Sales, Purchase, Bank)
	ModelAccountTax      Model = "account.tax"       // Taxes

	// Purchase Models
	ModelPurchaseOrder     Model = "purchase.order"      // Purchase Orders
	ModelPurchaseOrderLine Model = "purchase.order.line" // Lines within a Purchase Order

	// Human Resources & Payroll Models
	ModelHrEmployee   Model = "hr.employee"   // Employees
	ModelHrDepartment Model = "hr.department" // Departments
	ModelHrJob        Model = "hr.job"        // Job Positions
	ModelHrExpense    Model = "hr.expense"    // Employee Expenses

	// Project & Timesheet Models
	ModelProjectProject      Model = "project.project"       // Projects
	ModelProjectTask         Model = "project.task"          // Project Tasks
	ModelAccountAnalyticLine Model = "account.analytic.line" // Timesheets / Analytic Entries

	// User & System Models
	ModelResUsers           Model = "res.users"             // Users of the Odoo system
	ModelResCompany         Model = "res.company"           // Companies (for multi-company setups)
	ModelResCurrency        Model = "res.currency"          // Currencies
	ModelResCountry         Model = "res.country"           // Countries
	ModelIrModel            Model = "ir.model"              // Odoo Models Metadata (used for introspection)
	ModelIrAttachment       Model = "ir.attachment"         // File Attachments linked to records
	ModelIrActionsActWindow Model = "ir.actions.act_window" // Window Actions (how views are opened)
	ModelIrSequence         Model = "ir.sequence"           // Document Sequencing (e.g., for invoice numbers)

	// Messaging & Activity Models
	ModelMailActivity Model = "mail.activity" // Scheduled Activities (tasks, calls, meetings)
	ModelMailMessage  Model = "mail.message"  // Messages in the chatter (discussions, notes)
	ModelMailThread   Model = "mail.thread"   // Base model for models with chatter functionality
)

// DomainCondition represents a single element within an Odoo domain filter.
// It can be either a 3-element tuple [field, operator, value] for a condition,
// or a single string element for a logical operator like "|" or "&".
//
// Examples:
//
//	{"name", "=", "John Doe"} // A standard condition
//	{"|"}                    // A logical OR operator
type DomainCondition []interface{}

// Domain represents a collection of DomainCondition elements.
// This type is used to build complex filter expressions for Odoo RPC calls.
type Domain []DomainCondition

// ToRPC converts the Go-native Domain type into the []interface{} format
// expected by Odoo's RPC (Remote Procedure Call) for domain filters.
// This method is crucial for transforming type-safe Go structs into the
// generic list structure required by the Odoo RPC client.
//
// It specifically handles the transformation of logical operators (like "|" or "&")
// from a single-element slice (e.g., {"|"}) into a direct string element within
// the final RPC array (e.g., "|"), aligning with Odoo's expected domain structure.
func (d Domain) ToRPC() []interface{} { // ¡Aquí está el cambio clave!
	if d == nil {
		// Odoo typically expects an empty list for no filter conditions, not nil.
		return []interface{}{}
	}

	// Initialize the slice that will hold the Odoo RPC domain.
	// This slice will contain a mix of []interface{} for conditions and string for operators.
	var rpcDomain []interface{}

	for _, cond := range d {
		// Check if the DomainCondition is a single-element slice, which typically indicates a logical operator.
		if len(cond) == 1 {
			// Attempt to convert the single element to a string.
			if op, ok := cond[0].(string); ok {
				// If it's a string (like "|", "&"), append it directly as a string to the rpcDomain.
				rpcDomain = append(rpcDomain, op)
			} else {
				// If it's a single element but not a string (uncommon for Odoo operators),
				// append it as a slice to maintain its original structure.
				rpcDomain = append(rpcDomain, cond)
			}
		} else {
			// For standard conditions (e.g., {"field", "=", "value"}), append the entire slice.
			rpcDomain = append(rpcDomain, cond)
		}
	}

	return rpcDomain
}

// Fields represents a slice of field names to retrieve from Odoo.
// This type alias adds semantic meaning to a []string when used for Odoo fields.
type Fields []string

// ToRPC converts the Fields type to a []string suitable for Odoo RPC calls.
func (f Fields) ToRPC() []string {
	return []string(f)
}

// OdooContext represents the 'context' dictionary passed as an option
// in Odoo RPC calls. It allows custom key-value pairs that influence
// Odoo's server-side logic (e.g., language, timezone, active_test).
type OdooContext map[string]interface{}

// Options represents common keyword arguments for Odoo RPC methods.
// This struct simplifies specifying common options like limit, offset, order,
// and the Odoo-specific 'context'. For less common options, the 'Extra' map can be used.
type Options struct {
	Context OdooContext            `json:"context,omitempty"` // Odoo's context dictionary
	Limit   int                    `json:"limit,omitempty"`   // Maximum number of records to return
	Offset  int                    `json:"offset,omitempty"`  // Number of records to skip
	Order   string                 `json:"order,omitempty"`   // Field(s) to sort by (e.g., "name asc", "date desc,id asc")
	Extra   map[string]interface{} `json:"extra,omitempty"`   // For any other less common Odoo options
}

// ToRPC converts the Options struct into the map[string]interface{} format
// expected by Odoo's RPC.
func (o *Options) ToRPC() map[string]interface{} {
	if o == nil {
		// Return an empty map if options are nil, as Odoo expects this when no options are passed.
		return map[string]interface{}{}
	}

	rpcOptions := make(map[string]interface{})

	if len(o.Context) > 0 { // Use len() for map, as advised by staticcheck
		rpcOptions["context"] = o.Context
	}
	if o.Limit > 0 { // Odoo ignores limits <= 0, so only include if positive
		rpcOptions["limit"] = o.Limit
	}
	if o.Offset > 0 { // Odoo ignores offsets <= 0, so only include if positive
		rpcOptions["offset"] = o.Offset
	}
	if o.Order != "" {
		rpcOptions["order"] = o.Order
	}
	// Merge any extra options
	for k, v := range o.Extra {
		rpcOptions[k] = v
	}

	return rpcOptions
}

// Data is a custom type designed to facilitate the declaration of
// data for Odoo records. It's essentially a map where keys are field names
// (strings) and values can be of any type (interface{}).
//
// This allows you to define record fields and their values in a structured,
// readable way, similar to a JSON object or a Python dictionary.
type Data map[string]interface{}

// ToRPC converts the Data type to a map[string]interface{} suitable for Odoo RPC calls.
// This method is provided for consistency with other ToRPC methods, though
// a direct type assertion is often sufficient.
func (d Data) ToRPC() map[string]interface{} {
	return map[string]interface{}(d)
}

// parseOptions converts a slice of Options pointers into a single map[string]interface{}
// suitable for Odoo RPC keyword arguments (kwargs). It handles merging multiple options.
func (c *OdooClient) parseOptions(options ...*Options) map[string]interface{} {
	if len(options) == 0 || options[0] == nil {
		return map[string]interface{}{}
	}

	// Start with the first options map.
	// Subsequent options will override or merge with previous ones.
	// For simplicity, we just use the first options struct provided.
	// A more complex merger could iterate if multiple *Options were passed.
	return options[0].ToRPC()
}
