package acme

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"
)

func TestClient_NewAccount(t *testing.T) {
	errorTests := []struct {
		Name                 string
		OnlyReturnExisting   bool
		TermsOfServiceAgreed bool
		Contact              []string
	}{
		{
			Name:                 "fetching non-existing account",
			OnlyReturnExisting:   true,
			TermsOfServiceAgreed: true,
		},
		{
			Name:                 "not agreeing to terms of service",
			OnlyReturnExisting:   false,
			TermsOfServiceAgreed: false,
		},
		{
			Name:                 "bad contacts",
			OnlyReturnExisting:   false,
			TermsOfServiceAgreed: true,
			Contact:              []string{"this will fail"},
		},
	}
	for _, currentTest := range errorTests {
		key := makePrivateKey(t)
		_, err := testClient.NewAccount(key, currentTest.OnlyReturnExisting, currentTest.TermsOfServiceAgreed, currentTest.Contact...)
		if err == nil {
			t.Fatalf("expected error %s, got none", currentTest.Name)
		}
		acmeErr, ok := err.(Problem)
		if !ok {
			t.Fatalf("unknown error %s: %v", currentTest.Name, err)
		}
		if acmeErr.Type == "" {
			t.Fatalf("%s no acme error type present: %+v", currentTest.Name, acmeErr)
		}
	}
}

func TestClient_NewAccount2(t *testing.T) {
	existingKey := makePrivateKey(t)
	successTests := []struct {
		Name     string
		Existing bool
		Key      crypto.Signer
		Contact  []string
	}{
		{
			Name: "new account without contact",
		},
		{
			Name:    "new account with contact",
			Contact: []string{"mailto:test@test.com"},
		},
		{
			Name: "new account for fetching existing",
			Key:  existingKey,
		},
		{
			Name:     "fetching existing account",
			Key:      existingKey,
			Existing: true,
		},
	}
	for _, currentTest := range successTests {
		var key crypto.Signer
		if currentTest.Key != nil {
			key = currentTest.Key
		} else {
			key = makePrivateKey(t)
		}
		if _, err := testClient.NewAccount(key, currentTest.Existing, true, currentTest.Contact...); err != nil {
			t.Fatalf("unexpected error %s: %v", currentTest.Name, err)
		}
	}
}

func TestClient_UpdateAccount(t *testing.T) {
	account := makeAccount(t)
	contact := []string{"mailto:test@test.com"}
	updatedAccount, err := testClient.UpdateAccount(account, contact...)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !reflect.DeepEqual(updatedAccount.Contact, contact) {
		t.Fatalf("contact mismatch, expected: %v, got: %v", contact, updatedAccount.Contact)
	}
}

func TestClient_UpdateAccount2(t *testing.T) {
	account := makeAccount(t)
	updatedAccount, err := testClient.UpdateAccount(Account{PrivateKey: account.PrivateKey, URL: account.URL})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !reflect.DeepEqual(account, updatedAccount) {
		t.Fatalf("account and updated account mismatch, expected: %+v, got: %+v", account, updatedAccount)
	}

	_, err = testClient.UpdateAccount(Account{PrivateKey: account.PrivateKey})
	if err == nil {
		t.Fatalf("expected error, got none")
	}
}

type errSigner struct{}

func (es errSigner) Public() crypto.PublicKey {
	privKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	return privKey
}

func (es errSigner) Sign(io.Reader, []byte, crypto.SignerOpts) ([]byte, error) {
	return nil, errors.New("cannot sign key okeydokey")
}

func TestClient_AccountKeyChange(t *testing.T) {
	tests := []struct {
		name         string
		account      func() Account
		newKey       func() crypto.Signer
		expectsError bool
		errorStr     string
	}{
		{
			name: "bad key",
			account: func() Account {
				return Account{
					PrivateKey: errSigner{},
				}
			},
			newKey: func() crypto.Signer {
				return nil
			},
			expectsError: true,
			errorStr:     "unknown key type",
		},
		{
			name:    "success",
			account: func() Account { return makeAccount(t) },
			newKey:  func() crypto.Signer { return makePrivateKey(t) },
		},
		{
			name:         "bad signer",
			account:      func() Account { return makeAccount(t) },
			newKey:       func() crypto.Signer { return errSigner{} },
			expectsError: true,
			errorStr:     "inner jws",
		},
		{
			name: "bad post",
			account: func() Account {
				acct := makeAccount(t)
				acct.URL = "invalid"
				return acct
			},
			newKey:       func() crypto.Signer { return makePrivateKey(t) },
			expectsError: true,
			errorStr:     "malformed",
		},
	}

	for i, ct := range tests {
		account := ct.account()
		newKey := ct.newKey()
		accountNewKey, err := testClient.AccountKeyChange(account, newKey)
		if ct.expectsError && err == nil {
			t.Errorf("AccountKeyChange test %d %q expected error, got none", i, ct.name)
		}
		if !ct.expectsError && err != nil {
			t.Errorf("AccountKeyChange test %d %q expected no error, got: %v", i, ct.name, err)
		}
		if err != nil && ct.errorStr != "" && !strings.Contains(err.Error(), ct.errorStr) {
			t.Errorf("AccountKeyChange test %d %q error doesnt contain %q: %s", i, ct.name, ct.errorStr, err.Error())
		}

		if err != nil {
			continue
		}
		if accountNewKey.PrivateKey == account.PrivateKey {
			t.Fatalf("UpdateAccount test %d %q account key didnt change", i, ct.name)
		}
		if accountNewKey.PrivateKey != newKey {
			t.Fatalf("UpdateAccount test %d %q new key isnt set", i, ct.name)
		}
	}

}

func TestClient_DeactivateAccount(t *testing.T) {
	account := makeAccount(t)
	var err error
	account, err = testClient.DeactivateAccount(account)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if account.Status != "deactivated" {
		t.Fatalf("expected account deactivated, got: %s", account.Status)
	}
}

func TestClient_FetchOrderList(t *testing.T) {
	if testClientMeta.Software == clientBoulder {
		t.Skip("boulder doesnt support orders list: https://github.com/letsencrypt/boulder/issues/3335")
		return
	}

	tests := []struct {
		pre          func(acct *Account) bool
		post         func(*testing.T, Account, OrderList)
		expectsError bool
		errorStr     string
	}{
		{
			pre: func(acct *Account) bool {
				acct.Orders = ""
				return false
			},
			expectsError: true,
			errorStr:     "no order",
		},
		{
			pre: func(acct *Account) bool {
				*acct, _, _ = makeOrderFinalised(t, nil)
				return true
			},
			post: func(st *testing.T, account Account, list OrderList) {
				if len(list.Orders) != 1 {
					st.Fatalf("expected 1 orders, got: %d", len(list.Orders))
				}
			},
			expectsError: false,
		},
	}

	for i, ct := range tests {
		acct := makeAccount(t)
		if ct.pre != nil {
			update := ct.pre(&acct)
			if update {
				var err error
				acct, err = testClient.UpdateAccount(acct)
				if err != nil {
					panic(err)
				}
			}
		}
		list, err := testClient.FetchOrderList(acct)
		if ct.expectsError && err == nil {
			t.Errorf("order list test %d expected error, got none", i)
		}
		if !ct.expectsError && err != nil {
			t.Errorf("order list test %d expected no error, got: %v", i, err)
		}
		if err != nil && ct.errorStr != "" && !strings.Contains(err.Error(), ct.errorStr) {
			t.Errorf("order list test %d error doesnt contain %q: %s", i, ct.errorStr, err.Error())
		}
		if ct.post != nil {
			ct.post(t, acct, list)
		}
	}

}

func TestClient_NewAccountOptions(t *testing.T) {
	tests := []struct {
		name         string
		options      []NewAccountOptionFunc
		expectsError bool
		errorStr     string
	}{
		{
			name: "opt error func",
			options: []NewAccountOptionFunc{
				func(signer crypto.Signer, account *Account, request *NewAccountRequest, client Client) error {
					return errors.New("ALWAYS ERRORS")
				},
			},
			expectsError: true,
			errorStr:     "ALWAYS",
		},
	}

	for i, ct := range tests {
		key := makePrivateKey(t)
		_, err := testClient.NewAccountOptions(key, ct.options...)
		if ct.expectsError && err == nil {
			t.Errorf("NewAccountOptions test %d %q expected error, got none", i, ct.name)
		}
		if !ct.expectsError && err != nil {
			t.Errorf("NewAccountOptions test %d %q expected no error, got: %v", i, ct.name, err)
		}
	}
}
