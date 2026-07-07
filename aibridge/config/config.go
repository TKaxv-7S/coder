package config

import (
	"context"
	"time"

	"github.com/coder/coder/v2/aibridge/keypool"
)

const (
	ProviderAnthropic = "anthropic"
	ProviderOpenAI    = "openai"
	ProviderCopilot   = "copilot"
)

// Anthropic carries configuration for an Anthropic provider.
type Anthropic struct {
	// Name is the provider instance name. If empty, defaults to "anthropic".
	Name    string
	BaseURL string
	// KeyPool holds the centralized keys, with automatic key failover. BYOK
	// credentials are resolved per request from the incoming headers.
	KeyPool *keypool.Pool
	// WIF configures Workload Identity Federation. Mutually exclusive
	// with KeyPool; at most one of WIF and KeyPool may be set.
	WIF              *AnthropicWIF
	APIDumpDir       string
	CircuitBreaker   *CircuitBreaker
	SendActorHeaders bool
}

// AnthropicWIF carries configuration for Anthropic Workload Identity
// Federation. When set on the Anthropic provider, the gateway exchanges
// an OIDC identity token for a short-lived Anthropic access token
// instead of using static API keys.
type AnthropicWIF struct {
	// FederationRuleID is the tagged ID (fdrl_...) of the Anthropic
	// federation rule governing this exchange. Required.
	FederationRuleID string
	// OrganizationID is the UUID of the Anthropic organization.
	// Required.
	OrganizationID string
	// IdentityToken returns the OIDC identity token (JWT) presented
	// for exchange. It is invoked on every token exchange, so
	// implementations can serve rotating credentials (e.g. re-read a
	// projected service account token file) or mint tokens on demand.
	// Must be safe for concurrent use. Required.
	IdentityToken func(ctx context.Context) (string, error)
	// ServiceAccountID is the svac_... tagged ID of the target service
	// account. Required for SERVICE_ACCOUNT-target federation rules;
	// omitted from the exchange request only for USER-target rules,
	// where the principal derives from the JWT claims.
	ServiceAccountID string
	// WorkspaceID is the wrkspc_... tagged ID or the literal "default".
	// Required when the federation rule is enabled for more than one
	// workspace; when omitted the server selects the rule's sole
	// enabled workspace.
	WorkspaceID string
}

type AWSBedrock struct {
	Region                     string
	AccessKey, AccessKeySecret string
	Model, SmallFastModel      string
	// If set, requests will be sent to this URL instead of the default AWS Bedrock endpoint
	// (https://bedrock-runtime.{region}.amazonaws.com).
	// This is useful for routing requests through a proxy or for testing.
	BaseURL string
	// RoleARN, when set, is assumed via STS before calling Bedrock. The base
	// identity (static keys or the AWS SDK default credential chain, e.g.
	// IRSA / EKS Pod Identity / EC2 Instance Profile) signs the AssumeRole
	// call, and the resulting temporary credentials sign Bedrock requests.
	RoleARN string
	// ExternalID is sent as the STS external ID on the AssumeRole call.
	// It is meaningful only alongside RoleARN and must match the
	// sts:ExternalId condition on the target role's trust policy.
	ExternalID string
}

// OpenAI carries configuration for an OpenAI provider.
type OpenAI struct {
	// Name is the provider instance name. If empty, defaults to "openai".
	Name    string
	BaseURL string
	// KeyPool holds the centralized keys, with automatic key failover. BYOK
	// credentials are resolved per request from the incoming headers.
	KeyPool          *keypool.Pool
	APIDumpDir       string
	CircuitBreaker   *CircuitBreaker
	SendActorHeaders bool
}

type Copilot struct {
	// Name is the provider instance name. If empty, defaults to "copilot".
	Name           string
	BaseURL        string
	APIDumpDir     string
	CircuitBreaker *CircuitBreaker
}

// CircuitBreaker holds configuration for circuit breakers.
type CircuitBreaker struct {
	// MaxRequests is the maximum number of requests allowed in half-open state.
	MaxRequests uint32
	// Interval is the cyclic period of the closed state for clearing internal counts.
	Interval time.Duration
	// Timeout is how long the circuit stays open before transitioning to half-open.
	Timeout time.Duration
	// FailureThreshold is the number of consecutive failures that triggers the circuit to open.
	FailureThreshold uint32
	// IsFailure determines if a status code should count as a failure.
	// If nil, defaults to DefaultIsFailure.
	IsFailure func(statusCode int) bool
	// OpenErrorResponse returns the response body when the circuit is open.
	// This should match the provider's error format.
	OpenErrorResponse func() []byte
}

// DefaultCircuitBreaker returns sensible defaults for circuit breaker configuration.
func DefaultCircuitBreaker() CircuitBreaker {
	return CircuitBreaker{
		FailureThreshold: 5,
		Interval:         10 * time.Second,
		Timeout:          30 * time.Second,
		MaxRequests:      3,
	}
}
