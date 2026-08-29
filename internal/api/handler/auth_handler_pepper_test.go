package handler

// Pepper configuration was consolidated in #163: the wallet seed derivation
// (formerly `AuthHandler.deriveWalletSeed` and the dead `getPasskeyPepper()`
// env read) now lives entirely in `wallet.Service.DeriveWalletSeed`, which
// reads the single `config.SecurityConfig.WalletPepper` (env
// MOISTELLO_WALLET_PEPPER) wired through `cmd/api-server/main.go`. The
// redundant PasskeyPepper config field and MOISTELLO_PASSKEY_PEPPER env var
// were removed. Derivation unit tests live in
// internal/domain/wallet/service_test.go and the config binding is covered by
// config/config_test.go. There is no direct env access left in handlers.
