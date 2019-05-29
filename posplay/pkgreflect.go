// Code generated by github.com/ungerik/pkgreflect DO NOT EDIT.

package posplay

import "reflect"

var Types = map[string]reflect.Type{
	"Config":                                   reflect.TypeOf((*Config)(nil)).Elem(),
	"ConnectionHandler":                        reflect.TypeOf((*ConnectionHandler)(nil)).Elem(),
	"PairConnection":                           reflect.TypeOf((*PairConnection)(nil)).Elem(),
	"PairConnectionExtra":                      reflect.TypeOf((*PairConnectionExtra)(nil)).Elem(),
	"ReachLevelAchievementStrategy":            reflect.TypeOf((*ReachLevelAchievementStrategy)(nil)).Elem(),
	"Session":                                  reflect.TypeOf((*Session)(nil)).Elem(),
	"StubAchievementStrategy":                  reflect.TypeOf((*StubAchievementStrategy)(nil)).Elem(),
	"SubmitAchievementStrategy":                reflect.TypeOf((*SubmitAchievementStrategy)(nil)).Elem(),
	"TripDuringDisturbanceAchievementStrategy": reflect.TypeOf((*TripDuringDisturbanceAchievementStrategy)(nil)).Elem(),
	"VisitStationsAchievementStrategy":         reflect.TypeOf((*VisitStationsAchievementStrategy)(nil)).Elem(),
	"VisitThroughoutLineAchievementStrategy":   reflect.TypeOf((*VisitThroughoutLineAchievementStrategy)(nil)).Elem(),
}

var Functions = map[string]reflect.Value{
	"BaseURL":                     reflect.ValueOf(BaseURL),
	"ConfigureRouter":             reflect.ValueOf(ConfigureRouter),
	"DescriptionForRarity":        reflect.ValueOf(DescriptionForRarity),
	"DescriptionForXPTransaction": reflect.ValueOf(DescriptionForXPTransaction),
	"DoXPTransaction":             reflect.ValueOf(DoXPTransaction),
	"GetSession":                  reflect.ValueOf(GetSession),
	"Initialize":                  reflect.ValueOf(Initialize),
	"NewSession":                  reflect.ValueOf(NewSession),
	"RegisterDiscussionParticipationCallback": reflect.ValueOf(RegisterDiscussionParticipationCallback),
	"RegisterEventWinCallback":                reflect.ValueOf(RegisterEventWinCallback),
	"RegisterReport":                          reflect.ValueOf(RegisterReport),
	"RegisterTripFirstEdit":                   reflect.ValueOf(RegisterTripFirstEdit),
	"RegisterTripSubmission":                  reflect.ValueOf(RegisterTripSubmission),
	"RegisterXPTransaction":                   reflect.ValueOf(RegisterXPTransaction),
	"ReloadAchievements":                      reflect.ValueOf(ReloadAchievements),
	"ReloadTemplates":                         reflect.ValueOf(ReloadTemplates),
	"WeekStart":                               reflect.ValueOf(WeekStart),
}

var Variables = map[string]reflect.Value{
	"TheConnectionHandler": reflect.ValueOf(&TheConnectionHandler),
}

var Consts = map[string]reflect.Value{
	"CSRFcookieName":                reflect.ValueOf(CSRFcookieName),
	"CSRFfieldName":                 reflect.ValueOf(CSRFfieldName),
	"DEBUG":                         reflect.ValueOf(DEBUG),
	"GameTimezone":                  reflect.ValueOf(GameTimezone),
	"NicknameNameType":              reflect.ValueOf(NicknameNameType),
	"PairProcessLongevity":          reflect.ValueOf(PairProcessLongevity),
	"PlayersOnlyProfilePrivacy":     reflect.ValueOf(PlayersOnlyProfilePrivacy),
	"PosPlayVersion":                reflect.ValueOf(PosPlayVersion),
	"PrivateLBPrivacy":              reflect.ValueOf(PrivateLBPrivacy),
	"PrivateProfilePrivacy":         reflect.ValueOf(PrivateProfilePrivacy),
	"PublicLBPrivacy":               reflect.ValueOf(PublicLBPrivacy),
	"PublicProfilePrivacy":          reflect.ValueOf(PublicProfilePrivacy),
	"SessionName":                   reflect.ValueOf(SessionName),
	"UsernameDiscriminatorNameType": reflect.ValueOf(UsernameDiscriminatorNameType),
	"UsernameNameType":              reflect.ValueOf(UsernameNameType),
}
