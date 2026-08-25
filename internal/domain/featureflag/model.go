package featureflag

import "time"

// FeatureFlag represents a feature toggle in the system.
type FeatureFlag struct {
	ID          int       `json:"id" db:"id"`
	Flag        string    `json:"flag" db:"flag"`
	Enabled     bool      `json:"enabled" db:"enabled"`
	Description string    `json:"description" db:"description"`
	UpdatedAt   time.Time `json:"updatedAt" db:"updated_at"`
}