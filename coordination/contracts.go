package coordination

// ScheduleWakeupCommand tells an effect adapter to submit Event when the
// effect becomes available. The adapter supplies a stable ID when it is empty.
type ScheduleWakeupCommand struct {
	Event Event `json:"event"`
}
