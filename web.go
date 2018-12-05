package main

import (
	"net/http"

	"github.com/gorilla/mux"
	"github.com/underlx/disturbancesmlx/discordbot"
	"github.com/underlx/disturbancesmlx/posplay"
	"github.com/underlx/disturbancesmlx/utils"
	"github.com/underlx/disturbancesmlx/website"
)

// WebServer runs the web server
func WebServer() {
	router := mux.NewRouter().StrictSlash(true)

	webKeybox, present := secrets.GetBox("web")
	if !present {
		webLog.Fatal("Web keybox not present in keybox")
	}

	// main perturbacoes.pt website
	website.Initialize(rootSqalxNode, webKeybox, webLog, reportHandler, vehicleHandler, statsHandler)

	posplayKeybox, present := secrets.GetBox("posplay")
	if !present {
		webLog.Fatal("Posplay keybox not present in keybox")
	}

	// PosPlay sub-website
	posplay.Initialize(posplay.Config{
		Keybox:     posplayKeybox,
		Log:        posplayLog,
		Store:      website.SessionStore(),
		Node:       rootSqalxNode,
		PathPrefix: website.BaseURL() + "/posplay",
		GitCommit:  GitCommit})

	// this order is important. see https://github.com/gorilla/mux/issues/411 (still open at the time of writing)
	posplay.ConfigureRouter(router.PathPrefix("/posplay").Subrouter())
	website.ConfigureRouter(router.PathPrefix("/").Subrouter())

	channel, present := webKeybox.Get("discordInviteChannel")
	fallbackInvite, present2 := webKeybox.Get("discordFallbackInvite")
	if present && present2 {
		router.HandleFunc("/discord", inviteHandler(channel, fallbackInvite))
	}

	webLog.Println("Starting Web server...")

	server := http.Server{
		Addr:    ":8089",
		Handler: router,
	}

	err := server.ListenAndServe()
	if err != nil {
		webLog.Println(err)
	}
	webLog.Println("Web server terminated")
}

func inviteHandler(channelID, fallbackInviteURL string) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		invite, err := discordbot.CreateInvite(channelID, utils.GetClientIP(r))
		if err != nil {
			http.Redirect(w, r, fallbackInviteURL, http.StatusTemporaryRedirect)
			return
		}
		http.Redirect(w, r, "https://discord.gg/"+invite.Code, http.StatusTemporaryRedirect)
	}
}
