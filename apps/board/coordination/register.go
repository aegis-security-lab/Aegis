package boardcoordination

import "aegis/coordination"

// Register installs the Board-owned collaboration policy into the platform
// coordination registry. The platform has no built-in Board mode.
func Register(registry *coordination.Registry) error {
	items := []coordination.Mode{BoardAutonomy{}}
	for _, item := range items {
		if err := registry.Register(item); err != nil {
			return err
		}
	}
	return nil
}
