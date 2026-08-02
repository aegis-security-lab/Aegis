package control

import (
	"context"
	"fmt"

	"aegis/capability"
	"aegis/coordination"
)

func planExecutionCapabilities(ctx context.Context, manager *Manager, planner *coordination.CapabilityPlanner, coordinationID string, defaults, requested, systemRequired []capability.Ref, selection coordination.CapabilitySelection) (coordination.CapabilityDecision, error) {
	policy := coordination.CapabilityPolicy{}
	if manager != nil && manager.Coordination() != nil {
		binding, err := manager.Coordination().Binding(ctx, coordinationID)
		if err != nil {
			return coordination.CapabilityDecision{}, err
		}
		policy, err = coordination.CapabilityPolicyFromBinding(binding)
		if err != nil {
			return coordination.CapabilityDecision{}, err
		}
	}
	active := coordination.CapabilityPlanner{}
	if planner != nil {
		active = *planner
	}
	decision, err := active.Plan(coordination.CapabilityPlanRequest{
		Defaults: defaults, Requested: requested, SystemRequired: systemRequired,
		Selection: selection, Policy: policy,
	})
	if err != nil {
		return coordination.CapabilityDecision{}, fmt.Errorf("control: plan execution capabilities: %w", err)
	}
	return decision, nil
}
