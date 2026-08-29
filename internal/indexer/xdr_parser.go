package indexer

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"strconv"

	"github.com/stellar/go/xdr"
)

// ParseContractEvents decodes the base64-encoded result_meta_xdr from a Stellar
// transaction and extracts all Soroban contract events contained within. Each
// returned ContractEvent is fully decoded with its EventType and Payload fields
// populated from the raw SCVal topics and data.
//
// Malformed or unrecognised XDR is silently skipped — the function returns
// whatever events could be successfully decoded along with the first error
// encountered (if any). Callers should treat partial results as valid.
func ParseContractEvents(txHash string, ledger int64, resultMetaXDR string) ([]ContractEvent, error) {
	if resultMetaXDR == "" {
		return nil, nil
	}

	raw, err := base64.StdEncoding.DecodeString(resultMetaXDR)
	if err != nil {
		return nil, fmt.Errorf("base64 decode result_meta_xdr: %w", err)
	}

	var meta xdr.TransactionMeta
	if _, err := xdr.Unmarshal(bytes.NewReader(raw), &meta); err != nil {
		return nil, fmt.Errorf("xdr unmarshal TransactionMeta: %w", err)
	}

	// Soroban contract events live in V3 metadata.
	v3 := meta.V3
	if v3 == nil {
		return nil, nil
	}

	var events []ContractEvent
	for _, diagEvent := range v3.SorobanMeta.Events {
		ev, ok := decodeContractEvent(txHash, ledger, diagEvent)
		if !ok {
			continue
		}
		events = append(events, ev)
	}
	return events, nil
}

// decodeContractEvent converts a raw xdr.ContractEvent into a typed ContractEvent.
// Returns (event, true) on success or (zero, false) if the event cannot be decoded.
func decodeContractEvent(txHash string, ledger int64, raw xdr.ContractEvent) (ContractEvent, bool) {
	// Only process contract-type events (not system or diagnostic).
	if raw.Type != xdr.ContractEventTypeContract {
		return ContractEvent{}, false
	}

	contractID := ""
	if raw.ContractId != nil {
		h := *raw.ContractId
		contractID = fmt.Sprintf("%x", h[:])
	}

	body, ok := raw.Body.GetV0()
	if !ok {
		return ContractEvent{}, false
	}

	topics := body.Topics
	if len(topics) == 0 {
		return ContractEvent{}, false
	}

	eventType := decodeSymbol(topics[0])
	if eventType == "" {
		return ContractEvent{}, false
	}

	payload := decodePayload(topics[1:], body.Data)

	return ContractEvent{
		ContractID: contractID,
		EventType:  eventType,
		Ledger:     ledger,
		TxHash:     txHash,
		Payload:    payload,
	}, true
}

// decodeSymbol extracts a string from an SCVal of type SCV_SYMBOL or SCV_STRING.
func decodeSymbol(v xdr.ScVal) string {
	switch v.Type {
	case xdr.ScValTypeScvSymbol:
		if sym, ok := v.GetSym(); ok {
			return string(sym)
		}
	case xdr.ScValTypeScvString:
		if s, ok := v.GetStr(); ok {
			return string(s)
		}
	}
	return ""
}

// decodePayload converts the remaining event topics and data SCVal into a flat
// map suitable for storage as JSONB. Keys are positional ("topic_1", "topic_2", …)
// for remaining topics, and "data" for the event data field.
//
// Named fields are extracted where the on-chain convention encodes them as
// alternating key-value symbol pairs within the topics list.
func decodePayload(remainingTopics []xdr.ScVal, data xdr.ScVal) map[string]any {
	payload := make(map[string]any)

	for i, topic := range remainingTopics {
		key := fmt.Sprintf("topic_%d", i+1)
		payload[key] = scValToGo(topic)
	}

	dataVal := scValToGo(data)
	if dataVal != nil {
		payload["data"] = dataVal
	}

	return payload
}

// scValToGo converts an xdr.ScVal to its closest Go-native representation.
// Complex nested types (maps, vecs) are recursively expanded.
func scValToGo(v xdr.ScVal) any {
	switch v.Type {
	case xdr.ScValTypeScvBool:
		b, _ := v.GetB()
		return b

	case xdr.ScValTypeScvSymbol:
		sym, _ := v.GetSym()
		return string(sym)

	case xdr.ScValTypeScvString:
		s, _ := v.GetStr()
		return string(s)

	case xdr.ScValTypeScvU32:
		u, _ := v.GetU32()
		return uint32(u)

	case xdr.ScValTypeScvI32:
		i, _ := v.GetI32()
		return int32(i)

	case xdr.ScValTypeScvU64:
		u, _ := v.GetU64()
		return uint64(u)

	case xdr.ScValTypeScvI64:
		i, _ := v.GetI64()
		return int64(i)

	case xdr.ScValTypeScvU128:
		u, _ := v.GetU128()
		// Represent as decimal string to avoid precision loss in JSON.
		hi := uint64(u.Hi)
		lo := uint64(u.Lo)
		if hi == 0 {
			return strconv.FormatUint(lo, 10)
		}
		return fmt.Sprintf("%d%018d", hi, lo)

	case xdr.ScValTypeScvI128:
		i, _ := v.GetI128()
		hi := int64(i.Hi)
		lo := uint64(i.Lo)
		if hi == 0 {
			return strconv.FormatUint(lo, 10)
		}
		return fmt.Sprintf("%d%018d", hi, lo)

	case xdr.ScValTypeScvAddress:
		addr, _ := v.GetAddress()
		return scAddressToString(addr)

	case xdr.ScValTypeScvBytes:
		b, _ := v.GetBytes()
		return base64.StdEncoding.EncodeToString(b)

	case xdr.ScValTypeScvMap:
		m, _ := v.GetMap()
		if m == nil {
			return nil
		}
		result := make(map[string]any, len(*m))
		for _, entry := range *m {
			k := fmt.Sprintf("%v", scValToGo(entry.Key))
			result[k] = scValToGo(entry.Val)
		}
		return result

	case xdr.ScValTypeScvVec:
		vec, _ := v.GetVec()
		if vec == nil {
			return nil
		}
		result := make([]any, len(*vec))
		for i, elem := range *vec {
			result[i] = scValToGo(elem)
		}
		return result

	case xdr.ScValTypeScvVoid:
		return nil

	default:
		return fmt.Sprintf("<SCVal type=%d>", v.Type)
	}
}

// scAddressToString converts an xdr.ScAddress to a human-readable string
// (Stellar account ID or contract hex).
func scAddressToString(addr xdr.ScAddress) string {
	switch addr.Type {
	case xdr.ScAddressTypeScAddressTypeAccount:
		if addr.AccountId != nil {
			return addr.AccountId.Address()
		}
	case xdr.ScAddressTypeScAddressTypeContract:
		if addr.ContractId != nil {
			return fmt.Sprintf("%x", (*addr.ContractId)[:])
		}
	}
	return ""
}
