// godoo/client.go
package godoo

import (
	"context" // Import context
	"crypto/tls"
	"fmt"
	"log" // Kept for defaultLogger fallback, if needed, but not for direct use
	"net/http"
	"net/url"
	"time"

	"github.com/kolo/xmlrpc"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore" // Added for defaultLogger customization example
)

// LoggerEnv define los tipos de entorno para la configuración del logger.
type LoggerEnv string

const (
	// EnvDevelopment configura el logger para un entorno de desarrollo (salida legible y con colores básicos).
	EnvDevelopment LoggerEnv = "development"
	// EnvProduction configura el logger para un entorno de producción (salida JSON estructurada).
	EnvProduction LoggerEnv = "production"
)

// OdooClient represents the Odoo XML-RPC client.
// It holds all connection parameters and session state.
type OdooClient struct {
	url           string
	db            string
	username      string
	password      string
	uid           int64
	rpcClient     *xmlrpc.Client
	lastAuth      time.Time
	authTimeout   time.Duration
	skipTLSVerify bool
	httpClient    *http.Client
	logger        *zap.Logger
}

// createLogger crea una instancia de Zap logger basada en el entorno especificado.
func createLogger(env LoggerEnv) *zap.Logger {
	var cfg zap.Config
	if env == EnvDevelopment {
		cfg = zap.NewDevelopmentConfig()
		// Deshabilita el campo "caller" para logs más limpios en desarrollo
		cfg.EncoderConfig.CallerKey = ""
		// Deshabilita el stacktrace para logs de desarrollo, a menos que sea un Fatal.
		cfg.DisableStacktrace = true
		// Personaliza el color del nivel si es posible con Development
		// (Zap Development ya tiene colores básicos en terminales compatibles)
	} else { // Por defecto, o si es EnvProduction
		cfg = zap.NewProductionConfig()
		cfg.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
		cfg.EncoderConfig.LevelKey = "level"
		cfg.EncoderConfig.TimeKey = "time"
		cfg.EncoderConfig.CallerKey = "caller" // Habilita el caller en producción
		cfg.DisableStacktrace = false          // Habilita el stacktrace para errores en producción
	}

	logger, err := cfg.Build()
	if err != nil {
		// Fallback a un logger no-op si Zap falla en construir
		log.Printf("Failed to build Zap logger for env '%s', falling back to no-op logger: %v", env, err)
		return zap.NewNop()
	}
	return logger
}

// Option es una función que configura un OdooClient.
type Option func(*OdooClient)

// WithAuthTimeout establece el tiempo de espera de autenticación para OdooClient.
func WithAuthTimeout(d time.Duration) Option {
	return func(c *OdooClient) {
		c.authTimeout = d
	}
}

// WithSkipTLSVerify establece si se debe omitir la verificación de certificados TLS.
// ADVERTENCIA: No usar en producción.
func WithSkipTLSVerify(skip bool) Option {
	return func(c *OdooClient) {
		c.skipTLSVerify = skip
	}
}

// WithHTTPClient establece un *http.Client personalizado para OdooClient.
func WithHTTPClient(httpClient *http.Client) Option {
	return func(c *OdooClient) {
		c.httpClient = httpClient
	}
}

// WithLogger establece un logger de Zap personalizado para OdooClient.
// Si se usa esta opción, anula la configuración automática de entorno.
func WithLogger(logger *zap.Logger) Option {
	return func(c *OdooClient) {
		c.logger = logger
	}
}

// WithLoggerEnv establece la configuración del logger de Zap basada en el entorno.
// Si WithLogger se usa también, WithLogger tendrá prioridad.
func WithLoggerEnv(env LoggerEnv) Option {
	return func(c *OdooClient) {
		c.logger = createLogger(env)
	}
}

// New creates a new OdooClient instance with functional options.
func New(urlStr, db, username, password string, opts ...Option) (*OdooClient, error) {
	parsedURL, err := url.Parse(urlStr)
	if err != nil {
		return nil, fmt.Errorf("failed to parse Odoo URL: %w", err)
	}
	if parsedURL.Scheme != "https" && parsedURL.Scheme != "http" {
		return nil, fmt.Errorf("invalid Odoo URL scheme: %s, must be http or https", parsedURL.Scheme)
	}

	client := &OdooClient{
		url:         urlStr,
		db:          db,
		username:    username,
		password:    password,
		authTimeout: 6 * time.Hour,
		httpClient:  http.DefaultClient,
		logger:      createLogger(EnvProduction),
	}

	// Aplicar opciones
	for _, opt := range opts {
		opt(client)
	}

	// Aplicar skipTLSVerify al Transport del httpClient
	if client.skipTLSVerify {
		client.logger.Warn("ODOO_SKIP_TLS_VERIFY is enabled. TLS certificate verification will be skipped for Odoo connections. DO NOT USE IN PRODUCTION.",
			zap.String("component", "OdooClient"),
			zap.String("action", "New"),
		)
		if client.httpClient.Transport == nil {
			client.httpClient.Transport = &http.Transport{
				TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
			}
		} else if tr, ok := client.httpClient.Transport.(*http.Transport); ok {
			tr.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
		} else {
			client.logger.Warn("Cannot apply skipTLSVerify to a custom HTTP client's non-http.Transport. Manual configuration might be needed.",
				zap.String("component", "OdooClient"),
				zap.String("action", "New"),
				zap.String("transport_type", fmt.Sprintf("%T", client.httpClient.Transport)),
			)
		}
	}

	return client, nil
}

// authenticate connects to the Odoo server and authenticates the user.
// It is called internally by getConnection if the authentication is invalid.
// It now accepts a context.Context to allow for cancellation or timeouts.
func (c *OdooClient) authenticate(ctx context.Context) error {
	// Check for context cancellation before starting the authentication process.
	select {
	case <-ctx.Done():
		c.logger.Debug("Authentication cancelled before starting due to context",
			zap.Error(ctx.Err()),
			zap.String("op", "authenticate"),
		)
		return ctx.Err()
	default:
		// Context is not done, proceed.
	}

	// The xmlrpc.NewClient from 'kolo' expects a *http.Transport.
	// We need to extract it from the OdooClient's httpClient.
	var tr *http.Transport
	if c.httpClient.Transport == nil {
		// If no custom transport is set, use the default HTTP transport
		tr = http.DefaultTransport.(*http.Transport)
	} else if customTr, ok := c.httpClient.Transport.(*http.Transport); ok {
		tr = customTr
	} else {
		// If a non-http.Transport is set, we can't configure TLS verification directly.
		// Log a warning or return an error if this is a critical misconfiguration.
		c.logger.Warn("OdooClient's HTTP client has a non-standard Transport. TLS settings (like InsecureSkipVerify) might not apply.",
			zap.String("transport_type", fmt.Sprintf("%T", c.httpClient.Transport)),
			zap.String("op", "authenticate"),
		)
		// For now, proceed with the existing custom transport, hoping it handles TLS
		// or that skipTLSVerify was handled by the user's custom http.Client.
		// As a fallback, use http.DefaultTransport if the custom one is not *http.Transport
		// This might not be ideal if the user intended their custom RoundTripper to be used.
		tr = http.DefaultTransport.(*http.Transport)
	}

	commonURL := fmt.Sprintf("%s/xmlrpc/2/common", c.url)
	// The xmlrpc.NewClient internally creates an http.Client.
	// If we want the context's deadline to apply, we need to ensure this
	// internal http.Client has a timeout set *before* calling `Call`.
	// The 'kolo/xmlrpc' library *does not* expose a way to pass a context
	// directly into its `Call` method, nor does it let us inject a custom `http.Client`
	// with `http.Client.Do(req.WithContext(ctx))`.
	// So, while `ctx` is here, its primary use will be for pre-call checks
	// and for passing through to `getConnection`.
	// For actual in-flight cancellation/timeout of the RPC, the `http.Client`'s Timeout
	// property or a manual goroutine-select pattern would be needed.

	// A more robust way to handle context-aware HTTP requests with `kolo/xmlrpc`
	// would be to have its `Call` method accept a `context.Context` and use `http.NewRequestWithContext`.
	// Since it doesn't, the `ctx` here primarily serves to:
	// 1. Allow for early exit if the parent context is cancelled before the call starts.
	// 2. Potentially, to set an overall timeout on the `http.Client` *before* it's passed to xmlrpc.NewClient.
	// However, `xmlrpc.NewClient` creates its own http.Client internally, making this difficult.

	// To truly honor `ctx.Deadline` or `ctx.Done()`, you might need to:
	// a) Wrap the `commonRPCClient.Call` in a goroutine and use a `select` with `ctx.Done()`.
	// b) Modify the `kolo/xmlrpc` library (fork it) to accept context.
	// c) Use a different XML-RPC client library.

	// For now, let's just ensure we respect `ctx.Done()` *before* the blocking call.
	// If the context has a deadline, we could potentially set `commonRPCClient.SetTimeout(...)`
	// if `kolo/xmlrpc` supported it, but it doesn't.

	commonRPCClient, err := xmlrpc.NewClient(commonURL, tr)
	if err != nil {
		c.logger.Error("Failed to connect to Odoo common endpoint during authentication",
			zap.Error(err),
			zap.String("url", commonURL),
			zap.String("op", "authenticate"),
		)
		return fmt.Errorf("failed to connect to Odoo common endpoint: %w", err)
	}
	defer commonRPCClient.Close() // Close the common client after use

	var uid int64
	err = commonRPCClient.Call("authenticate", []interface{}{c.db, c.username, c.password, map[string]interface{}{}}, &uid)
	if err != nil {
		c.logger.Error("Odoo authentication failed",
			zap.Error(err),
			zap.String("db", c.db),
			zap.String("username", c.username),
			zap.String("op", "authenticate"),
		)
		// Consider using the specific error types defined in godoo/errors.go
		return fmt.Errorf("%w: %s", ErrAuthenticationFailed, err.Error())
	}

	// Check for context cancellation after the first RPC call (authenticate) but before the next.
	select {
	case <-ctx.Done():
		c.logger.Debug("Authentication cancelled after first RPC call due to context",
			zap.Error(ctx.Err()),
			zap.String("op", "authenticate"),
		)
		return ctx.Err()
	default:
		// Context is not done, proceed.
	}

	objectURL := fmt.Sprintf("%s/xmlrpc/2/object", c.url)
	objectRPCClient, err := xmlrpc.NewClient(objectURL, tr)
	if err != nil {
		c.logger.Error("Failed to connect to Odoo object endpoint after authentication",
			zap.Error(err),
			zap.String("url", objectURL),
			zap.String("op", "authenticate"),
		)
		return fmt.Errorf("failed to connect to Odoo object endpoint: %w", err)
	}
	// Do not close objectRPCClient here, as it's stored and reused

	c.uid = uid
	c.rpcClient = objectRPCClient // Store the client for later use.
	c.lastAuth = time.Now()
	c.logger.Info("Successfully authenticated with Odoo",
		zap.Int64("uid", c.uid),
		zap.String("db", c.db),
		zap.String("op", "authenticate"),
	)
	return nil
}

// isAuthValid checks if the current authentication is valid (not expired and client exists).
func (c *OdooClient) isAuthValid() bool {
	return c.uid != 0 && c.rpcClient != nil && time.Since(c.lastAuth) < c.authTimeout
}

// getConnection returns the user ID and the RPC client, authenticating if necessary.
// It now accepts a context.Context to allow for cancellation or timeouts during connection.
func (c *OdooClient) getConnection(ctx context.Context) (int64, *xmlrpc.Client, error) {
	// Check for context cancellation before proceeding
	select {
	case <-ctx.Done():
		c.logger.Debug("Context cancelled before getting Odoo connection", zap.Error(ctx.Err()))
		return 0, nil, ctx.Err()
	default:
		// Continue
	}

	if !c.isAuthValid() {
		if c.rpcClient != nil {
			c.rpcClient.Close()
			c.rpcClient = nil
		}
		// Pass the context to the authentication process
		if err := c.authenticate(ctx); err != nil {
			return 0, nil, err
		}
	}
	return c.uid, c.rpcClient, nil
}
