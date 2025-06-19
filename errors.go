// godoo/errors.go
package godoo

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// Common Odoo client specific errors.
var (
	// ErrAuthenticationFailed indica que la autenticación con Odoo falló.
	ErrAuthenticationFailed = errors.New("godoo: authentication failed")

	// ErrRecordNotFound indica que no se encontró ningún registro para los criterios dados
	// en operaciones como SearchOne o ReadOne.
	ErrRecordNotFound = errors.New("godoo: no record found for the given criteria")

	// ErrInvalidModel indica que el modelo de Odoo especificado no existe o es inválido.
	ErrInvalidModel = errors.New("godoo: invalid Odoo model")

	// ErrInvalidMethod indica que el método especificado no existe para el modelo de Odoo dado.
	ErrInvalidMethod = errors.New("godoo: invalid Odoo method for the model")

	// ErrOdooRPC es un error genérico para cualquier fallo en la llamada XML-RPC a Odoo,
	// cuando no se puede clasificar más específicamente. El error subyacente de la librería XML-RPC
	// estará envuelto.
	ErrOdooRPC = errors.New("godoo: Odoo XML-RPC call failed")

	// ErrInvalidResponse is returned when the Odoo RPC response is
	// malformed or not in the expected format.
	ErrInvalidResponse = errors.New("invalid Odoo RPC response")
)

// OdooRPCError representa un error más estructurado devuelto por el servidor Odoo XML-RPC.
// Envuelve el error original del cliente XML-RPC.
type OdooRPCError struct {
	OriginalError error  // El error subyacente de la librería xmlrpc
	Code          int    // Código de error de Odoo (si se puede parsear, a menudo 0 o -32xxx)
	Message       string // Mensaje de error de Odoo
}

// Error implementa la interfaz error para OdooRPCError.
func (e *OdooRPCError) Error() string {
	if e.OriginalError != nil {
		return fmt.Sprintf("%s: %s (original: %v)", ErrOdooRPC, e.Message, e.OriginalError)
	}
	return fmt.Sprintf("%s: %s", ErrOdooRPC, e.Message)
}

// Unwrap permite el uso de errors.Is y errors.As con OdooRPCError.
func (e *OdooRPCError) Unwrap() error {
	return e.OriginalError
}

// parseOdooRPCError intenta analizar un error genérico del cliente XML-RPC
// para devolver un error más específico de godoo.
// Esto es crucial porque la librería 'kolo/xmlrpc' a menudo devuelve errores como simples strings.
func parseOdooRPCError(err error) error {
	if err == nil {
		return nil
	}

	errMsg := err.Error()

	// Intenta extraer el código y mensaje de error de un "XML-RPC fault"
	// Ejemplo: "XML-RPC fault: <Fault 1: 'Access denied'>"
	re := regexp.MustCompile(`Fault (\d+): '(.*?)'`)
	matches := re.FindStringSubmatch(errMsg)

	var faultCode int
	var faultMessage string = errMsg // Por defecto, el mensaje completo

	if len(matches) == 3 {
		if code, cerr := strconv.Atoi(matches[1]); cerr == nil {
			faultCode = code
		}
		faultMessage = matches[2]
	} else if strings.HasPrefix(errMsg, "XML-RPC fault: ") {
		// En caso de que sea un Fault pero no coincida el regex (menos común)
		faultMessage = strings.TrimPrefix(errMsg, "XML-RPC fault: ")
	}

	// Heurísticas para errores específicos de Odoo basadas en el mensaje
	// Estas verificaciones deben ir ANTES de retornar el error genérico OdooRPCError,
	// para que podamos devolver un tipo de error más preciso.

	// Error de modelo inválido
	if strings.Contains(faultMessage, "The model does not exist") ||
		strings.Contains(faultMessage, "No model named") ||
		strings.Contains(faultMessage, "not found in registry") ||
		(strings.Contains(faultMessage, "'object' object has no attribute") && strings.Contains(faultMessage, "model")) {
		return fmt.Errorf("%w: %s (original: %w)", ErrInvalidModel, faultMessage, err)
	}

	// Error de método inválido
	if strings.Contains(faultMessage, "Object has no method") ||
		strings.Contains(faultMessage, "method does not exist") ||
		(strings.Contains(faultMessage, "missing 1 required positional argument") && strings.Contains(faultMessage, "self")) {
		return fmt.Errorf("%w: %s (original: %w)", ErrInvalidMethod, faultMessage, err)
	}

	// Si no se detecta un error más específico, devuelve el error genérico OdooRPCError
	// con la información parseada.
	return &OdooRPCError{
		OriginalError: err,
		Code:          faultCode,
		Message:       faultMessage,
	}
}
