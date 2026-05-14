package router

// DecisionReason represents why the router made a decision.
type DecisionReason string

const (
	// Respond reasons
	ReasonMention              DecisionReason = "mention"
	ReasonReply                DecisionReason = "reply"
	ReasonNameMention          DecisionReason = "name_mention"
	ReasonActiveConversation   DecisionReason = "active_conversation"

	// Ignore reasons
	ReasonExceptionChannel  DecisionReason = "exception_channel"
	ReasonChannelNotAllowed DecisionReason = "channel_not_allowed"
	ReasonBotMessage        DecisionReason = "bot_message"
	ReasonNoTrigger         DecisionReason = "no_trigger"
)

// Decision represents the router's decision on whether to respond to an event.
type Decision struct {
	// Should indicates whether the bot should respond.
	Should bool

	// Reason explains why the decision was made.
	Reason DecisionReason
}

// Respond returns a Decision to respond with the given reason.
func Respond(reason DecisionReason) *Decision {
	return &Decision{
		Should: true,
		Reason: reason,
	}
}

// Ignore returns a Decision to ignore with the given reason.
func Ignore(reason DecisionReason) *Decision {
	return &Decision{
		Should: false,
		Reason: reason,
	}
}
