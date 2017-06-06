package dataobjects

import (
	"errors"
	"fmt"
	"time"

	"sort"

	sq "github.com/gbl08ma/squirrel"
	"github.com/heetch/sqalx"
	"github.com/lib/pq"
)

// Network is a transportation network
type Network struct {
	ID           string
	Name         string
	TypicalCars  int
	Holidays     []int64
	OpenTime     Time
	OpenDuration Duration
}

// GetNetworks returns a slice with all registered networks
func GetNetworks(node sqalx.Node) ([]*Network, error) {
	networks := []*Network{}

	tx, err := node.Beginx()
	if err != nil {
		return networks, err
	}
	defer tx.Commit() // read-only tx

	rows, err := sdb.Select("id", "name", "typ_cars", "holidays", "open_time", "open_duration").
		From("network").RunWith(tx).Query()
	if err != nil {
		return networks, fmt.Errorf("GetNetworks: %s", err)
	}
	defer rows.Close()

	for rows.Next() {
		var network Network
		var holidays pq.Int64Array
		err := rows.Scan(
			&network.ID,
			&network.Name,
			&network.TypicalCars,
			&holidays,
			&network.OpenTime,
			&network.OpenDuration)
		if err != nil {
			return networks, fmt.Errorf("GetNetworks: %s", err)
		}
		network.Holidays = holidays
		networks = append(networks, &network)
	}
	if err := rows.Err(); err != nil {
		return networks, fmt.Errorf("GetNetworks: %s", err)
	}
	return networks, nil
}

// GetNetwork returns the Network with the given ID
func GetNetwork(node sqalx.Node, id string) (*Network, error) {
	var network Network
	tx, err := node.Beginx()
	if err != nil {
		return &network, err
	}
	defer tx.Commit() // read-only tx

	var holidays pq.Int64Array
	err = sdb.Select("id", "name", "typ_cars", "holidays", "open_time", "open_duration").
		From("network").
		Where(sq.Eq{"id": id}).
		RunWith(tx).QueryRow().Scan(&network.ID, &network.Name, &network.TypicalCars, &holidays, &network.OpenTime, &network.OpenDuration)
	if err != nil {
		return &network, errors.New("GetNetwork: " + err.Error())
	}
	network.Holidays = holidays
	return &network, nil
}

// Lines returns the lines in this network
func (network *Network) Lines(node sqalx.Node) ([]*Line, error) {
	s := sdb.Select().
		Where(sq.Eq{"network": network.ID})
	return getLinesWithSelect(node, s)
}

// Stations returns the stations in this network
func (network *Network) Stations(node sqalx.Node) ([]*Station, error) {
	s := sdb.Select().
		Where(sq.Eq{"network": network.ID})
	return getStationsWithSelect(node, s)
}

// LastDisturbance returns the latest disturbance affecting this network
func (network *Network) LastDisturbance(node sqalx.Node) (*Disturbance, error) {
	tx, err := node.Beginx()
	if err != nil {
		return nil, err
	}
	defer tx.Commit() // read-only tx
	lines, err := network.Lines(tx)
	if err != nil {
		return nil, errors.New("LastDisturbance: " + err.Error())
	}
	lastDisturbances := []*Disturbance{}
	for _, line := range lines {
		d, err := line.LastDisturbance(tx)
		if err != nil {
			continue
		}
		lastDisturbances = append(lastDisturbances, d)
	}
	if len(lastDisturbances) == 0 {
		return nil, errors.New("No disturbances for this network")
	}
	sort.Slice(lastDisturbances, func(iidx, jidx int) bool {
		i := lastDisturbances[iidx]
		j := lastDisturbances[jidx]
		// i < j ?
		if i.Ended && j.Ended {
			return i.EndTime.Before(j.EndTime)
		}
		if i.Ended && !j.Ended {
			return true
		}
		if !i.Ended && j.Ended {
			return false
		}
		return i.StartTime.Before(j.StartTime)
	})
	return lastDisturbances[len(lastDisturbances)-1], nil
}

// CountDisturbancesByHour counts disturbances by hour between the specified dates
func (network *Network) CountDisturbancesByHour(node sqalx.Node, start time.Time, end time.Time) ([]int, error) {
	tx, err := node.Beginx()
	if err != nil {
		return []int{}, err
	}
	defer tx.Commit() // read-only tx

	rows, err := tx.Query("SELECT curd, COUNT(id) "+
		"FROM generate_series(date_trunc('hour', $2 at time zone $1), date_trunc('hour', $3 at time zone $1), '1 hour') AS curd "+
		"LEFT OUTER JOIN line_disturbance ON "+
		"(curd BETWEEN date_trunc('hour', time_start at time zone $1) AND date_trunc('hour', coalesce(time_end, now()) at time zone $1)) "+
		"GROUP BY curd ORDER BY curd;",
		start.Location().String(), start, end)
	if err != nil {
		return []int{}, fmt.Errorf("CountDisturbancesByHour: %s", err)
	}
	defer rows.Close()

	var counts []int
	for rows.Next() {
		var date time.Time
		var count int
		err := rows.Scan(&date, &count)
		if err != nil {
			return counts, fmt.Errorf("CountDisturbancesByHour: %s", err)
		}
		if err != nil {
			return counts, fmt.Errorf("CountDisturbancesByHour: %s", err)
		}
		counts = append(counts, count)
	}
	if err := rows.Err(); err != nil {
		return counts, fmt.Errorf("CountDisturbancesByHour: %s", err)
	}
	return counts, nil
}

// Update adds or updates the network
func (network *Network) Update(node sqalx.Node) error {
	tx, err := node.Beginx()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	_, err = sdb.Insert("network").
		Columns("id", "name", "typ_cars", "holidays", "open_time", "open_duration").
		Values(network.ID, network.Name, network.TypicalCars, pq.Int64Array(network.Holidays), network.OpenTime, network.OpenDuration).
		Suffix("ON CONFLICT (id) DO UPDATE SET name = ?, typ_cars = ?, holidays = ?, open_time = ?, open_duration = ?",
			network.Name, network.TypicalCars, network.Holidays, network.OpenTime, network.OpenDuration).
		RunWith(tx).Exec()

	if err != nil {
		return errors.New("AddNetwork: " + err.Error())
	}
	return tx.Commit()
}

// Delete deletes the network
func (network *Network) Delete(node sqalx.Node) error {
	tx, err := node.Beginx()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	_, err = sdb.Delete("network").
		Where(sq.Eq{"id": network.ID}).RunWith(tx).Exec()
	if err != nil {
		return fmt.Errorf("RemoveNetwork: %s", err)
	}
	return tx.Commit()
}
