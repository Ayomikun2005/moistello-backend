package chat

import "time"

// Conversation is a 1:1 encrypted chat session between two users. UserAID
// is always the lexicographically smaller of the two user IDs so a given
// pair of users maps to exactly one conversation regardless of who
// initiated it.
type Conversation struct {
	ID        string    `json:"id" db:"id"`
	UserAID   string    `json:"userAId" db:"user_a_id"`
	UserBID   string    `json:"userBId" db:"user_b_id"`
	CreatedAt time.Time `json:"createdAt" db:"created_at"`
}

// OtherParticipant returns the ID of whichever participant isn't userID.
func (c Conversation) OtherParticipant(userID string) string {
	if c.UserAID == userID {
		return c.UserBID
	}
	return c.UserAID
}

// HasParticipant reports whether userID is one of the two conversation
// participants.
func (c Conversation) HasParticipant(userID string) bool {
	return c.UserAID == userID || c.UserBID == userID
}

// Message is one ciphertext envelope in a conversation. The server never
// sees plaintext: Ciphertext/Nonce are opaque, client-encrypted blobs.
// EphemeralKey/OneTimePreKeyID are only set on the first message of a
// session (see 041_create_chat migration).
type Message struct {
	ID              string    `json:"id" db:"id"`
	ConversationID  string    `json:"conversationId" db:"conversation_id"`
	SenderID        string    `json:"senderId" db:"sender_id"`
	Sequence        int64     `json:"sequence" db:"sequence"`
	Ciphertext      string    `json:"ciphertext" db:"ciphertext"`
	Nonce           string    `json:"nonce" db:"nonce"`
	EphemeralKey    *string   `json:"ephemeralKey,omitempty" db:"ephemeral_key"`
	OneTimePreKeyID *int      `json:"oneTimePreKeyId,omitempty" db:"one_time_prekey_id"`
	CreatedAt       time.Time `json:"createdAt" db:"created_at"`
}
