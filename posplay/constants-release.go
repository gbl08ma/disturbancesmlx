// +build release

package posplay

import "time"

const (
	// DEBUG is whether this is a debug build
	DEBUG = false

	// SessionName is the name of the PosPlay session in the session store
	SessionName = "posplay"

	// GameTimezone is the timezone where the game is played
	GameTimezone = "Europe/Lisbon"

	// CSRFfieldName is the name of the form field used for CSRF protection
	CSRFfieldName = "posplay.csrf"

	// CSRFcookieName is the name of the cookie used for CSRF protection
	CSRFcookieName = "_posplay_csrf"

	// PairProcessLongevity sets the timeout for pairing a device with a PosPlay account
	PairProcessLongevity = 5 * time.Minute

	// PosPlayVersion is the version of this PosPlay subsystem release
	PosPlayVersion = "v0.10"
)
