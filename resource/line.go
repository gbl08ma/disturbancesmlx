package resource

import (
	"github.com/gbl08ma/sqalx"
	"github.com/underlx/disturbancesmlx/types"
	"github.com/yarf-framework/yarf"
)

// Line composites resource
type Line struct {
	resource
}

type apiLine struct {
	ID          string               `msgpack:"id" json:"id"`
	Name        string               `msgpack:"name" json:"name"`
	MainLocale  string               `msgpack:"mainLocale" json:"mainLocale"`
	Names       map[string]string    `msgpack:"names" json:"names"`
	Color       string               `msgpack:"color" json:"color"`
	TypicalCars int                  `msgpack:"typCars" json:"typCars"`
	Order       int                  `msgpack:"order" json:"order"`
	Network     *types.Network `msgpack:"-" json:"-"`
	ExternalID  string               `msgpack:"externalID" json:"externalID"`
}

type apiLineSchedule struct {
	Line         *types.Line    `msgpack:"-" json:"-"`
	Holiday      bool                 `msgpack:"holiday" json:"holiday"`
	Day          int                  `msgpack:"day" json:"day"`
	Open         bool                 `msgpack:"open" json:"open"`
	OpenTime     types.Time     `msgpack:"openTime" json:"openTime"`
	OpenDuration types.Duration `msgpack:"duration" json:"duration"`
}

type apiLinePath struct {
	ID   string       `msgpack:"id" json:"id"`
	Path [][2]float64 `msgpack:"path" json:"path"`
}

type apiLineWrapper struct {
	apiLine   `msgpack:",inline"`
	NetworkID string            `msgpack:"network" json:"network"`
	Stations  []string          `msgpack:"stations" json:"stations"`
	Schedule  []apiLineSchedule `msgpack:"schedule" json:"schedule"`
	Paths     []apiLinePath     `msgpack:"worldPaths" json:"worldPaths"`
}

// WithNode associates a sqalx Node with this resource
func (r *Line) WithNode(node sqalx.Node) *Line {
	r.node = node
	return r
}

// Get serves HTTP GET requests on this resource
func (r *Line) Get(c *yarf.Context) error {
	tx, err := r.Beginx()
	if err != nil {
		return err
	}
	defer tx.Commit() // read-only tx

	var lines []*types.Line
	if c.Param("id") == "conditions" {
		// yarf's router is a bit limited
		nc := yarf.NewContext(c.Request, c.Response)
		return new(LineCondition).WithNode(r.node).Get(nc)
	} else if c.Param("id") != "" {
		line, err := types.GetLine(tx, c.Param("id"))
		if err != nil {
			return err
		}
		lines = []*types.Line{line}
	} else {
		lines, err = types.GetLines(tx)
		if err != nil {
			return err
		}
	}

	apilines := make([]apiLineWrapper, len(lines))
	for i := range lines {
		apilines[i] = apiLineWrapper{
			apiLine:   apiLine(*lines[i]),
			NetworkID: lines[i].Network.ID,
		}

		apilines[i].Stations = []string{}
		stations, err := lines[i].Stations(tx)
		if err != nil {
			return err
		}
		for _, station := range stations {
			apilines[i].Stations = append(apilines[i].Stations, station.ID)
		}

		apilines[i].Schedule = []apiLineSchedule{}
		schedules, err := lines[i].Schedules(tx)
		if err != nil {
			return err
		}
		for _, s := range schedules {
			apilines[i].Schedule = append(apilines[i].Schedule, apiLineSchedule(*s))
		}

		apilines[i].Paths = []apiLinePath{}
		paths, err := lines[i].Paths(tx)
		if err != nil {
			return err
		}
		for _, p := range paths {
			apiPath := apiLinePath{
				ID: p.ID,
			}
			apiPath.Path = [][2]float64{}
			for _, point := range p.Path.P {
				apiPath.Path = append(apiPath.Path, [2]float64{point.X, point.Y})
			}
			apilines[i].Paths = append(apilines[i].Paths, apiPath)
		}
	}

	if c.Param("id") != "" {
		RenderData(c, apilines[0], "s-maxage=10")
	} else {
		RenderData(c, apilines, "s-maxage=10")
	}
	return nil
}
