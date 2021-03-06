package compute

import (
	"errors"
	"math"
	"strconv"
	"sync"
	"time"

	"github.com/gbl08ma/sqalx"
	cache "github.com/patrickmn/go-cache"
	uuid "github.com/satori/go.uuid"
	"github.com/underlx/disturbancesmlx/types"
)

// ReportHandler implements resource.ReportHandler
type ReportHandler struct {
	reports        *cache.Cache
	statsHandler   *StatsHandler
	node           sqalx.Node
	thresholds     *sync.Map
	multiplier     float32
	baseOffset     int
	statusReporter func(status *types.Status, allowNotify bool)
}

// NewReportHandler initializes a new ReportHandler and returns it
func NewReportHandler(statsHandler *StatsHandler, node sqalx.Node,
	statusReporter func(status *types.Status, allowNotify bool)) *ReportHandler {
	h := &ReportHandler{
		reports:        cache.New(cache.NoExpiration, 30*time.Second),
		statsHandler:   statsHandler,
		node:           node,
		thresholds:     new(sync.Map),
		multiplier:     1,
		statusReporter: statusReporter,
	}
	h.reports.OnEvicted(func(string, interface{}) {
		h.evaluateSituation()
	})
	return h
}

type reportData struct {
	Report types.Report
	Weight int
}

// HandleLineDisturbanceReport handles line disturbance reports
func (r *ReportHandler) HandleLineDisturbanceReport(report *types.LineDisturbanceReport) error {
	if closed, err := report.Line().CurrentlyClosed(r.node); err == nil && closed {
		return errors.New("HandleLineDisturbanceReport: the line of this report is currently closed")
	}

	weight, err := r.getVoteWeightForReport(report)
	if err != nil {
		return err
	}

	return r.AddReportManually(report, weight)
}

// AddReportManually forcefully adds a report with a manually specified weight (works even on closed lines)
func (r *ReportHandler) AddReportManually(report *types.LineDisturbanceReport, weight int) error {
	data := &reportData{report, weight}
	err := r.reports.Add(report.RateLimiterKey(), data, 15*time.Minute)
	if err != nil {
		return errors.New("HandleLineDisturbanceReport: report rate-limited")
	}
	go r.evaluateSituation()
	return nil
}

func (r *ReportHandler) getVoteWeightForReport(report *types.LineDisturbanceReport) (int, error) {
	if !report.ReplayProtected() || report.Submitter() == nil {
		return 1, nil
	}

	// app user that is currently in the reported line
	if r.statsHandler.UserInLine(report.Line(), report.Submitter()) {
		return 30, nil
	}

	// app user that is currently in the reported network
	if r.statsHandler.UserInNetwork(report.Line().Network, report.Submitter()) {
		return 20, nil
	}

	// app user that submitted a trip in the last 20 minutes
	recentTrips, err := types.GetTripsForSubmitterBetween(r.node, report.Submitter(), time.Now().Add(-20*time.Minute), time.Now())
	if err != nil {
		return 0, err
	}
	if len(recentTrips) > 0 {
		return 10, nil
	}

	// app user that is not in the network/has location turned off
	return 5, nil
}

func (r *ReportHandler) getEarliestVoteForLine(line *types.Line) *reportData {
	earliestTime := time.Time{}
	var earliestValue *reportData
	for _, item := range r.reports.Items() {
		data := item.Object.(*reportData)
		if ldr, ok := data.Report.(*types.LineDisturbanceReport); ok {
			if earliestTime.IsZero() || ldr.Time().Before(earliestTime) {
				earliestTime = ldr.Time()
				earliestValue = data
			}
		}
	}
	return earliestValue
}

// CountVotesForLine counts how many votes there are for a disturbance in this line
func (r *ReportHandler) CountVotesForLine(line *types.Line) int {
	count := 0
	for _, item := range r.reports.Items() {
		data := item.Object.(*reportData)
		if ldr, ok := data.Report.(*types.LineDisturbanceReport); ok {
			if ldr.Line().ID == line.ID {
				count += data.Weight
			}
		}
	}
	return count
}

// ClearVotesForLine clears reports for the specified line
func (r *ReportHandler) ClearVotesForLine(line *types.Line) {
	for key, item := range r.reports.Items() {
		data := item.Object.(*reportData)
		if ldr, ok := data.Report.(*types.LineDisturbanceReport); ok {
			if ldr.Line().ID == line.ID {
				r.reports.Delete(key)
			}
		}
	}
}

// GetThresholdForLine returns the current threshold for the specified line
func (r *ReportHandler) GetThresholdForLine(line *types.Line) int {
	numUsers := r.statsHandler.OITInLine(line, 0)
	var newValue int
	if numUsers <= 1 {
		newValue = 15
	} else {
		newValue = int(math.Round(56.8206*math.Log(float64(numUsers)) - 18.9))
	}

	data, _ := r.thresholds.LoadOrStore(line.ID, cache.New(5*time.Minute, 5*time.Minute))
	cache := data.(*cache.Cache)
	// only store one value per minute
	cache.SetDefault(strconv.FormatInt(time.Now().Unix()/60, 16), newValue)

	sum := 0
	count := 0
	for _, item := range cache.Items() {
		sum += item.Object.(int)
		count++
	}
	return r.baseOffset + int((float32(sum)/float32(count))*r.multiplier)
}

func (r *ReportHandler) lineHasEnoughVotesToStartDisturbance(line *types.Line) bool {
	return r.CountVotesForLine(line) >= r.GetThresholdForLine(line)
}

func (r *ReportHandler) lineHasEnoughVotesToKeepDisturbance(line *types.Line) bool {
	return r.CountVotesForLine(line) >= r.GetThresholdForLine(line)/2
}

func (r *ReportHandler) evaluateSituation() {
	tx, err := r.node.Beginx()
	if err != nil {
		mainLog.Println("ReportHandler: " + err.Error())
		return
	}
	defer tx.Commit() // read-only tx (any new statuses are handled in different transactions)

	lines, err := types.GetLines(tx)
	if err != nil {
		mainLog.Println("ReportHandler: " + err.Error())
		return
	}

	for _, line := range lines {
		disturbances, err := line.OngoingDisturbances(tx, false)
		if err != nil {
			mainLog.Println("ReportHandler: " + err.Error())
			return
		}

		if len(disturbances) == 0 && r.lineHasEnoughVotesToStartDisturbance(line) {
			err := r.startDisturbance(tx, line)
			if err != nil {
				mainLog.Println("ReportHandler: " + err.Error())
			}
			continue
		}

		// the system works in such a way that there can only be one ongoing disturbance per line at a time,
		// but we use a loop anyway
		for _, disturbance := range disturbances {
			if disturbance.Official {
				// this avoids a new disturbance reopening immediately after it officially ends
				r.ClearVotesForLine(line)
			} else if !r.lineHasEnoughVotesToKeepDisturbance(line) {
				// end this unofficial disturbance
				err := r.endDisturbance(line)
				if err != nil {
					mainLog.Println("ReportHandler: " + err.Error())
				}
			}
		}
	}
}

func (r *ReportHandler) startDisturbance(node sqalx.Node, line *types.Line) error {
	earliestVote := r.getEarliestVoteForLine(line)
	if earliestVote == nil {
		return errors.New("earliest vote is nil")
	}

	latestDisturbance, err := line.LastDisturbance(node, false)
	if err != nil {
		return err
	}

	id, err := uuid.NewV4()
	if err != nil {
		return err
	}

	if earliestVote.Report.Time().After(latestDisturbance.UEndTime) {
		// even though we are only creating the disturbance now, the start time might be the time of the earliest report in memory
		// we would then add two line states: one for the date of the earliest report ("users began reporting...")
		// (we do not notify for this first state)
		// and another for the current time ("reports have been confirmed by multiple users...")

		if earliestVote.Report.Time().Before(time.Now().Add(-1 * time.Minute)) {
			// avoid issuing two states if their times would be too close to each other
			status := &types.Status{
				ID:         id.String(),
				Time:       earliestVote.Report.Time().UTC(),
				Line:       line,
				IsDowntime: true,
				Status:     "Os utilizadores comunicaram problemas na circulação",
				Source: &types.Source{
					ID:        "underlx-community",
					Name:      "UnderLX user community",
					Automatic: false,
					Official:  false,
				},
				MsgType: types.ReportBeginMessage,
			}

			r.statusReporter(status, false)

			id, err = uuid.NewV4()
			if err != nil {
				return err
			}
		}

		status := &types.Status{
			ID:         id.String(),
			Time:       time.Now().UTC(),
			Line:       line,
			IsDowntime: true,
			Status:     "Vários utilizadores confirmaram problemas na circulação",
			Source: &types.Source{
				ID:        "underlx-community",
				Name:      "UnderLX user community",
				Automatic: false,
				Official:  false,
			},
			MsgType: types.ReportConfirmMessage,
		}

		r.statusReporter(status, true)
	} else {
		// last disturbance ended after the earliest vote we have in memory
		// show a different message in this case

		status := &types.Status{
			ID:         id.String(),
			Time:       time.Now().UTC(),
			Line:       line,
			IsDowntime: true,
			Status:     "Vários utilizadores confirmaram mais problemas na circulação",
			Source: &types.Source{
				ID:        "underlx-community",
				Name:      "UnderLX user community",
				Automatic: false,
				Official:  false,
			},
			MsgType: types.ReportReconfirmMessage,
		}

		r.statusReporter(status, true)
	}
	return nil
}

func (r *ReportHandler) endDisturbance(line *types.Line) error {
	id, err := uuid.NewV4()
	if err != nil {
		return err
	}

	status := &types.Status{
		ID:         id.String(),
		Time:       time.Now().UTC(),
		Line:       line,
		IsDowntime: false,
		Status:     "Já não existem relatos de problemas na circulação",
		Source: &types.Source{
			ID:        "underlx-community",
			Name:      "UnderLX user community",
			Automatic: false,
			Official:  false,
		},
		MsgType: types.ReportSolvedMessage,
	}

	r.statusReporter(status, true)
	return nil
}

// ThresholdMultiplier returns the current threshold multiplier
func (r *ReportHandler) ThresholdMultiplier() float32 {
	return r.multiplier
}

// SetThresholdMultiplier sets the current threshold multiplier
func (r *ReportHandler) SetThresholdMultiplier(m float32) {
	if m > 0 {
		r.multiplier = m
	}
}

// ThresholdOffset returns the current threshold offset
func (r *ReportHandler) ThresholdOffset() int {
	return r.baseOffset
}

// SetThresholdOffset sets the current threshold offset
func (r *ReportHandler) SetThresholdOffset(offset int) {
	r.baseOffset = offset
}
