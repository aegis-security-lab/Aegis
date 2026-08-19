package control

import "testing"

func bridgeTestManager(t *testing.T) (*Store, *Manager) {
	t.Helper()
	store := configuredStore(t)
	manager, err := NewManager(store)
	if err != nil {
		t.Fatal(err)
	}
	_, phone, err := NewControlAgentPhone(manager)
	if err != nil {
		t.Fatal(err)
	}
	manager.SetAgentPhoneClient(phone)
	t.Cleanup(manager.Close)
	return store, manager
}
