package stellar

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/moistello/backend/pkg/tracing"
	"github.com/rs/zerolog/log"
	"go.opentelemetry.io/otel/attribute"
)

type Client struct {
	horizonURL        string
	sorobanRPCURL     string
	networkPassphrase string
	httpClient        *http.Client
	cb                *CircuitBreaker
}

func NewClient(horizonURL, sorobanRPCURL, networkPassphrase string) *Client {
	return &Client{
		horizonURL:        horizonURL,
		sorobanRPCURL:     sorobanRPCURL,
		networkPassphrase: networkPassphrase,
		httpClient:        &http.Client{Timeout: 30 * time.Second},
		cb:                NewCircuitBreaker("horizon", DefaultCircuitBreakerConfig()),
	}
}

type HorizonAccountResponse struct {
	ID       string `json:"id"`
	Sequence string `json:"sequence"`
	Balances []struct {
		Balance     string `json:"balance"`
		AssetType   string `json:"asset_type"`
		AssetCode   string `json:"asset_code"`
		AssetIssuer string `json:"asset_issuer"`
	} `json:"balances"`
}

func (c *Client) GetAccount(ctx context.Context, address string) (account *HorizonAccountResponse, err error) {
	ctx, span := tracing.StartStellarSpan(ctx, "get_account")
	start := time.Now()
	defer func() { tracing.EndSpan(span, err, start, attribute.String("stellar.address", address)) }()

	err = c.cb.Execute(ctx, func() error {
		url := fmt.Sprintf("%s/accounts/%s", c.horizonURL, address)
		resp, err := c.httpClient.Get(url)
		if err != nil {
			return fmt.Errorf("horizon request: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode == http.StatusNotFound {
			return fmt.Errorf("account not found")
		}
		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			return fmt.Errorf("horizon error %d: %s", resp.StatusCode, string(body))
		}

		var a HorizonAccountResponse
		if err := json.NewDecoder(resp.Body).Decode(&a); err != nil {
			return fmt.Errorf("decoding horizon response: %w", err)
		}
		account = &a
		return nil
	})
	if err != nil {
		return nil, err
	}
	return account, nil
}

func (c *Client) GetTransaction(ctx context.Context, txnHash string) (result map[string]any, err error) {
	ctx, span := tracing.StartStellarSpan(ctx, "get_transaction")
	start := time.Now()
	defer func() { tracing.EndSpan(span, err, start, attribute.String("stellar.txn_hash", txnHash)) }()

	err = c.cb.Execute(ctx, func() error {
		url := fmt.Sprintf("%s/transactions/%s", c.horizonURL, txnHash)
		resp, err := c.httpClient.Get(url)
		if err != nil {
			return err
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("horizon error %d", resp.StatusCode)
		}

		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (c *Client) VerifyTransaction(ctx context.Context, txnHash string, expectedFrom string, expectedAmount string) (bool, error) {
	// Ensure transaction exists and was successful
	txn, err := c.GetTransaction(ctx, txnHash)
	if err != nil {
		log.Warn().Err(err).Str("txn", txnHash).Msg("failed to fetch transaction")
		return false, err
	}
	if success, ok := txn["successful"].(bool); ok && !success {
		return false, nil
	}

	// Fetch operations for the transaction and look for a matching payment
	url := fmt.Sprintf("%s/transactions/%s/operations?limit=200", c.horizonURL, txnHash)
	resp, err := c.httpClient.Get(url)
	if err != nil {
		return false, fmt.Errorf("horizon request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return false, fmt.Errorf("horizon error %d: %s", resp.StatusCode, string(body))
	}

	var ops struct {
		Embedded struct {
			Records []map[string]any `json:"records"`
		} `json:"_embedded"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&ops); err != nil {
		return false, fmt.Errorf("decoding horizon operations: %w", err)
	}

	for _, r := range ops.Embedded.Records {
		typ, _ := r["type"].(string)
		if typ != "payment" && typ != "payment_strict_receive" && typ != "payment_strict_send" {
			continue
		}
		to, _ := r["to"].(string)
		amount, _ := r["amount"].(string)
		from, _ := r["from"].(string)

		// If expectedFrom is provided, ensure operation source matches
		if expectedFrom != "" && from != expectedFrom {
			// skip if source doesn't match expectedFrom when provided
			continue
		}

		if amount == expectedAmount && (expectedFrom == "" || to == expectedFrom || from == expectedFrom) {
			return true, nil
		}
		if expectedFrom != "" && to == expectedFrom && amount == expectedAmount {
			return true, nil
		}
	}

	return false, nil
}

func (c *Client) NetworkPassphrase() string { return c.networkPassphrase }
func (c *Client) HorizonURL() string        { return c.horizonURL }
func (c *Client) SorobanRPCURL() string     { return c.sorobanRPCURL }
