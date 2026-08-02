package modes

import "aegis/coordination"

func RegisterBuiltins(registry *coordination.Registry) error {
	items := []coordination.Mode{BoardAutonomy{}}
	for _, item := range items {
		if err := registry.Register(item); err != nil {
			return err
		}
	}
	return nil
}
