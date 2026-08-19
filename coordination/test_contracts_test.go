package coordination

const (
	testEventSubjectAssigned     EventType = "subject_assigned"
	testEventWaitRequested       EventType = "wait_requested"
	testEventMessageReceived     EventType = "message_received"
	testEventDelegationRequested EventType = "delegation_requested"

	testEffectEnqueueSubject EffectType = "enqueue_subject"
	testEffectSendMessage    EffectType = "send_message"
	testEffectStartWorker    EffectType = "start_worker"
	testEffectResumeActor    EffectType = "resume_actor"
)

type testDeliveryCommand struct {
	ActorID string `json:"actorId"`
	Message string `json:"message"`
}
