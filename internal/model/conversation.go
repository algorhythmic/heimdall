package model

import "heimdall/internal/conversation"

// Conversation contains observational metadata only. Evidence bytes live in SQL.
type Conversation = conversation.Record
