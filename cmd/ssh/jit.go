package ssh

import (
	"fmt"

	"github.com/fosrl/cli/internal/api"
	"github.com/fosrl/cli/internal/config"
	"github.com/fosrl/cli/internal/sshkeys"
)

// GenerateAndSignKey generates an Ed25519 key pair and signs the public key via the API.
// Returns private key (PEM), public key (authorized_keys line), certificate, and sign response data. No files are written.
func GenerateAndSignKey(client *api.Client, orgID string, resourceID int) (privPEM, pubKey, cert string, signData *api.SignSSHKeyData, err error) {
	privPEM, pubKey, err = sshkeys.GenerateKeyPair()
	if err != nil {
		return "", "", "", nil, fmt.Errorf("generate key pair: %w", err)
	}

	data, err := client.SignSSHKey(orgID, api.SignSSHKeyRequest{
		PublicKey:  pubKey,
		ResourceID: resourceID,
	})
	if err != nil {
		return "", "", "", nil, fmt.Errorf("sign SSH key: %w", err)
	}

	return privPEM, pubKey, data.Certificate, data, nil
}

// ResolveOrgID returns orgID from the flag or the active account. Returns empty string and nil error if both are empty.
func ResolveOrgID(accountStore *config.AccountStore, flagOrgID string) (string, error) {
	if flagOrgID != "" {
		return flagOrgID, nil
	}
	active, err := accountStore.ActiveAccount()
	if err != nil || active == nil {
		return "", errOrgRequired
	}
	if active.OrgID == "" {
		return "", errOrgRequired
	}
	return active.OrgID, nil
}
