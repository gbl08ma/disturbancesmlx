package posplay

import (
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/underlx/disturbancesmlx/dataobjects"
	"github.com/underlx/disturbancesmlx/discordbot"

	"github.com/gbl08ma/sqalx"
	"github.com/gorilla/csrf"
	"github.com/gorilla/mux"
	uuid "github.com/satori/go.uuid"
	"github.com/underlx/disturbancesmlx/utils"
)

// pageCommons contains information that is required by most page templates
type pageCommons struct {
	PageTitle   string
	DebugBuild  bool
	Session     *Session
	Player      *dataobjects.PPPlayer
	CSRFfield   template.HTML
	VersionInfo string

	// sidebar
	SidebarSelected  string
	AvatarURL        string
	XP               int
	Level            int
	LevelProgression float64
	XPthisWeek       int
	RankThisWeek     int
}

// ConfigureRouter configures a router to handle PosPlay paths
func ConfigureRouter(router *mux.Router) {
	router.HandleFunc("/", homePage)
	router.HandleFunc("/pair", pairPage)
	router.HandleFunc("/pair/status", pairStatus)
	router.HandleFunc("/settings", settingsPage)
	router.HandleFunc("/leaderboards", leaderboardsPage)
	router.HandleFunc("/leaderboards/weekly", leaderboardsPage)
	router.HandleFunc("/leaderboards/alltime", leaderboardsAllTimePage)
	router.HandleFunc("/login", forceLogin)
	router.HandleFunc("/logout", forceLogout)
	router.HandleFunc("/oauth/callback", callbackHandler)
	router.HandleFunc("/privacy", privacyPolicyPage)
	if DEBUG {
		router.Use(templateReloadingMiddleware)
	}
	router.Use(csrfMiddleware)
}

// ReloadTemplates reloads the templates for the website
func ReloadTemplates() {
	funcMap := template.FuncMap{
		"minus": func(a, b int) int {
			return a - b
		},
		"plus": func(a, b int) int {
			return a + b
		},
		"minus64": func(a, b int64) int64 {
			return a - b
		},
		"plus64": func(a, b int64) int64 {
			return a + b
		},
		"stringContains": func(s, substr string) bool {
			return strings.Contains(s, substr)
		},
		"formatTime": func(t time.Time) string {
			loc, _ := time.LoadLocation(GameTimezone)
			return t.In(loc).Format("02 Jan 2006 15:04")
		},
		"uuid": func() string {
			id, err := uuid.NewV4()
			if err == nil {
				return id.String()
			}
			return ""
		},
		"xpTxDescription":            descriptionForXPTransaction,
		"formatPortugueseMonth":      utils.FormatPortugueseMonth,
		"getDisplayNameFromNameType": getDisplayNameFromNameType,
		"formatLeaderboardWeek": func(start time.Time) string {
			end := start.AddDate(0, 0, 6)
			year, week := start.ISOWeek()
			return fmt.Sprintf("%dª semana de %d (%d %s - %d %s)",
				week, year,
				start.Day(), utils.FormatPortugueseMonthShort(start.Month()),
				end.Day(), utils.FormatPortugueseMonthShort(end.Month()))
		},
		"userAvatarURL": userAvatarURL,
	}

	webtemplate = template.Must(template.New("index.html").Funcs(funcMap).ParseGlob("templates/posplay/*.html"))
}

func templateReloadingMiddleware(next http.Handler) http.Handler {
	ReloadTemplates()
	return next
}

// initPageCommons fills PageCommons with the info that is required by most page templates
func initPageCommons(node sqalx.Node, w http.ResponseWriter, r *http.Request, title string, session *Session, player *dataobjects.PPPlayer) (commons pageCommons, err error) {
	commons.PageTitle = title + " | PosPlay"
	commons.DebugBuild = DEBUG
	commons.Session = session
	commons.Player = player
	commons.CSRFfield = csrf.TemplateField(r)
	commons.VersionInfo = PosPlayVersion + "-" + config.GitCommit

	if player != nil && node != nil {
		tx, err := node.Beginx()
		if err != nil {
			return commons, err
		}
		defer tx.Commit() // read-only tx

		commons.XP, commons.Level, commons.LevelProgression, err = player.Level(tx)
		if err != nil {
			return commons, err
		}

		commons.XPthisWeek, err = player.XPBalanceBetween(tx, getWeekStart(), time.Now())
		if err != nil {
			return commons, err
		}

		commons.RankThisWeek, err = player.RankBetween(tx, getWeekStart(), time.Now())
		if err != nil {
			return commons, err
		}

		commons.AvatarURL = session.DiscordInfo.AvatarURL("256")
	}

	return commons, nil
}

func homePage(w http.ResponseWriter, r *http.Request) {
	session, _, err := GetSession(r, w, false)
	if err != nil {
		config.Log.Println(err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	if session != nil {
		dashboardPage(w, r, session)
		return
	}

	p := struct {
		pageCommons
	}{}
	p.pageCommons, err = initPageCommons(nil, w, r, "Página principal", session, nil)
	if err != nil {
		config.Log.Println(err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	err = webtemplate.ExecuteTemplate(w, "index.html", p)
	if err != nil {
		config.Log.Println(err)
		w.WriteHeader(http.StatusInternalServerError)
	}
}

func dashboardPage(w http.ResponseWriter, r *http.Request, session *Session) {
	tx, err := config.Node.Beginx()
	if err != nil {
		config.Log.Println(err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	defer tx.Commit() // read-only tx

	player, err := dataobjects.GetPPPlayer(tx, uidConvS(session.DiscordInfo.ID))
	if err != nil {
		config.Log.Println(err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	p := struct {
		pageCommons

		XPTransactions []*dataobjects.PPXPTransaction
		JoinedServer   bool
		PairedDevice   bool
	}{
		JoinedServer: player.InGuild,
	}
	p.pageCommons, err = initPageCommons(tx, w, r, "Página principal", session, player)
	if err != nil {
		config.Log.Println(err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	p.SidebarSelected = "home"

	p.XPTransactions, err = player.XPTransactionsLimit(tx, 10)
	if err != nil {
		config.Log.Println(err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	_, err = dataobjects.GetPPPair(tx, player.DiscordID)
	p.PairedDevice = err == nil

	err = webtemplate.ExecuteTemplate(w, "dashboard.html", p)
	if err != nil {
		config.Log.Println(err)
		w.WriteHeader(http.StatusInternalServerError)
	}
}

func pairPage(w http.ResponseWriter, r *http.Request) {
	session, redirected, err := GetSession(r, w, true)
	if err != nil {
		config.Log.Println(err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	if redirected {
		return
	}

	tx, err := config.Node.Beginx()
	if err != nil {
		config.Log.Println(err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	defer tx.Commit() // read-only

	discordID := uidConvS(session.DiscordInfo.ID)

	player, err := dataobjects.GetPPPlayer(tx, discordID)
	if err != nil {
		config.Log.Println(err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	p := struct {
		pageCommons
		PairProcess *pairProcess
		CurrentPair *dataobjects.PPPair
	}{
		PairProcess: TheConnectionHandler.getProcess(discordID),
	}
	p.pageCommons, err = initPageCommons(tx, w, r, "Associação com dispositivo", session, player)
	if err != nil {
		config.Log.Println(err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	p.SidebarSelected = "pair"

	p.CurrentPair, _ = dataobjects.GetPPPair(tx, discordID)

	err = webtemplate.ExecuteTemplate(w, "pair.html", p)
	if err != nil {
		config.Log.Println(err)
		w.WriteHeader(http.StatusInternalServerError)
	}
}

func pairStatus(w http.ResponseWriter, r *http.Request) {
	session, _, err := GetSession(r, w, false)
	if err != nil {
		config.Log.Println(err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	if session == nil {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	pairProcess := TheConnectionHandler.getProcess(uidConvS(session.DiscordInfo.ID))

	data := make(map[string]interface{})
	data["completed"] = pairProcess.Completed
	data["expiry"] = pairProcess.Expires.Unix()
	data["code"] = pairProcess.Code

	b, err := json.Marshal(data)
	if err != nil {
		config.Log.Println(err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.Write(b)
}

func settingsPage(w http.ResponseWriter, r *http.Request) {
	session, redirected, err := GetSession(r, w, true)
	if err != nil {
		config.Log.Println(err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	if redirected {
		return
	}

	tx, err := config.Node.Beginx()
	if err != nil {
		config.Log.Println(err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	defer tx.Rollback()

	player, err := dataobjects.GetPPPlayer(tx, uidConvS(session.DiscordInfo.ID))
	if err != nil {
		config.Log.Println(err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	p := struct {
		pageCommons
		JoinedServer  bool
		GuildMember   *discordgo.Member
		SavedSettings bool
	}{
		JoinedServer: player.InGuild,
	}
	p.pageCommons, err = initPageCommons(tx, w, r, "Definições", session, player)
	if err != nil {
		config.Log.Println(err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	p.SidebarSelected = "settings"
	p.GuildMember, err = discordbot.ProjectGuildMember(session.DiscordInfo.ID)
	if err != nil {
		p.GuildMember = nil
	}

	if r.Method == http.MethodPost {
		r.ParseForm()
		switch r.Form.Get("name-preference") {
		case "username-discriminator":
			player.NameType = UsernameDiscriminatorNameType
		case "username":
			player.NameType = UsernameNameType
		case "nickname":
			player.NameType = NicknameNameType
		}

		switch r.Form.Get("lbprivacy-preference") {
		case "public":
			player.LBPrivacy = PublicLBPrivacy
		case "private":
			player.LBPrivacy = PrivateLBPrivacy
		}

		player.CachedName = getDisplayNameFromNameType(player.NameType, session.DiscordInfo, p.GuildMember)

		err = refreshSession(r, w, session, p.GuildMember, player)
		if err != nil {
			config.Log.Println(err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		err = player.Update(tx)
		if err != nil {
			config.Log.Println(err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
	}

	p.SavedSettings = r.Method == http.MethodPost

	err = webtemplate.ExecuteTemplate(w, "settings.html", p)
	if err != nil {
		config.Log.Println(err)
		w.WriteHeader(http.StatusInternalServerError)
	}

	err = tx.Commit()
	if err != nil {
		config.Log.Println(err)
		w.WriteHeader(http.StatusInternalServerError)
	}
}

func leaderboardsPage(w http.ResponseWriter, r *http.Request) {
	session, redirected, err := GetSession(r, w, true)
	if err != nil {
		config.Log.Println(err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	if redirected {
		return
	}

	tx, err := config.Node.Beginx()
	if err != nil {
		config.Log.Println(err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	defer tx.Commit() // read-only

	discordID := uidConvS(session.DiscordInfo.ID)

	player, err := dataobjects.GetPPPlayer(tx, discordID)
	if err != nil {
		config.Log.Println(err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	p := struct {
		pageCommons
		Leaderboards []struct {
			Start   time.Time
			Entries []dataobjects.PPLeaderboardEntry
		}
	}{}
	p.pageCommons, err = initPageCommons(tx, w, r, "Classificações", session, player)
	if err != nil {
		config.Log.Println(err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	p.SidebarSelected = "leaderboards"

	start := getWeekStart()
	end := time.Now()
	for i := 0; i < 5; i++ {
		entries, err := dataobjects.PPLeaderboardBetween(tx, start, end, 15, player)
		if err != nil {
			config.Log.Println(err)
			w.WriteHeader(http.StatusInternalServerError)
		}
		if len(entries) == 1 && entries[0].Position == 0 {
			// avoid showing just this player in the 0th place
			entries = []dataobjects.PPLeaderboardEntry{}
		}

		p.Leaderboards = append(p.Leaderboards, struct {
			Start   time.Time
			Entries []dataobjects.PPLeaderboardEntry
		}{
			Start:   start,
			Entries: entries,
		})

		end = start
		start = start.AddDate(0, 0, -7)
	}

	err = webtemplate.ExecuteTemplate(w, "leaderboards.html", p)
	if err != nil {
		config.Log.Println(err)
		w.WriteHeader(http.StatusInternalServerError)
	}
}

func leaderboardsAllTimePage(w http.ResponseWriter, r *http.Request) {
	session, redirected, err := GetSession(r, w, true)
	if err != nil {
		config.Log.Println(err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	if redirected {
		return
	}

	tx, err := config.Node.Beginx()
	if err != nil {
		config.Log.Println(err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	defer tx.Commit() // read-only

	discordID := uidConvS(session.DiscordInfo.ID)

	player, err := dataobjects.GetPPPlayer(tx, discordID)
	if err != nil {
		config.Log.Println(err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	p := struct {
		pageCommons
		Leaderboard struct {
			Entries []dataobjects.PPLeaderboardEntry
		}
	}{}
	p.pageCommons, err = initPageCommons(tx, w, r, "Classificações globais", session, player)
	if err != nil {
		config.Log.Println(err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	p.SidebarSelected = "leaderboards"

	start := time.Time{}
	end := time.Now()
	entries, err := dataobjects.PPLeaderboardBetween(tx, start, end, 50, player)
	if err != nil {
		config.Log.Println(err)
		w.WriteHeader(http.StatusInternalServerError)
	}
	if len(entries) == 1 && entries[0].Position == 0 {
		// avoid showing just this player in the 0th place
		entries = []dataobjects.PPLeaderboardEntry{}
	}

	p.Leaderboard = struct {
		Entries []dataobjects.PPLeaderboardEntry
	}{
		Entries: entries,
	}

	err = webtemplate.ExecuteTemplate(w, "leaderboards-alltime.html", p)
	if err != nil {
		config.Log.Println(err)
		w.WriteHeader(http.StatusInternalServerError)
	}
}

func privacyPolicyPage(w http.ResponseWriter, r *http.Request) {
	session, _, err := GetSession(r, w, false)
	if err != nil {
		config.Log.Println(err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	tx, err := config.Node.Beginx()
	if err != nil {
		config.Log.Println(err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	defer tx.Commit() // read-only tx

	p := struct {
		pageCommons
	}{}
	p.pageCommons, err = initPageCommons(tx, w, r, "Política de Privacidade", session, nil)
	if err != nil {
		config.Log.Println(err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	err = webtemplate.ExecuteTemplate(w, "privacy.html", p)
	if err != nil {
		config.Log.Println(err)
		w.WriteHeader(http.StatusInternalServerError)
	}
}

func forceLogin(w http.ResponseWriter, r *http.Request) {
	_, redirected, err := GetSession(r, w, true)
	if err != nil {
		config.Log.Println(err)
		w.WriteHeader(http.StatusInternalServerError)
	}
	if !redirected {
		http.Redirect(w, r, config.PathPrefix, http.StatusTemporaryRedirect)
	}
}

func forceLogout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	session, _, err := GetSession(r, w, false)
	if err != nil {
		config.Log.Println(err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	if session != nil {
		session.Logout(r, w)
	}
	http.Redirect(w, r, config.PathPrefix, http.StatusTemporaryRedirect)
}
