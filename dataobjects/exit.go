package dataobjects

import (
	"errors"
	"fmt"

	sq "github.com/gbl08ma/squirrel"
	"github.com/heetch/sqalx"
	"github.com/lib/pq"
)

// Exit is a Lobby exit
type Exit struct {
	ID         int
	WorldCoord [2]float64
	Streets    []string
	Type       string
	Lobby      *Lobby
}

// GetExits returns a slice with all registered exits
func GetExits(node sqalx.Node) ([]*Exit, error) {
	return getExitsWithSelect(node, sdb.Select())
}

func getExitsWithSelect(node sqalx.Node, sbuilder sq.SelectBuilder) ([]*Exit, error) {
	exits := []*Exit{}

	tx, err := node.Beginx()
	if err != nil {
		return exits, err
	}
	defer tx.Commit() // read-only tx

	rows, err := sbuilder.Columns("id", "lobby_id", "world_coord", "streets", "type").
		From("station_lobby_exit").
		RunWith(tx).Query()
	if err != nil {
		return exits, fmt.Errorf("getExitsWithSelect: %s", err)
	}
	defer rows.Close()

	var lobbyIDs []string
	for rows.Next() {
		var exit Exit
		var lobbyID string
		var worldCoord Point
		var streets pq.StringArray
		err := rows.Scan(
			&exit.ID,
			&lobbyID,
			&worldCoord,
			&streets,
			&exit.Type)
		if err != nil {
			return exits, fmt.Errorf("getExitsWithSelect: %s", err)
		}
		exit.WorldCoord[0] = worldCoord[0]
		exit.WorldCoord[1] = worldCoord[1]
		exit.Streets = streets
		exits = append(exits, &exit)
		lobbyIDs = append(lobbyIDs, lobbyID)
	}
	if err := rows.Err(); err != nil {
		return exits, fmt.Errorf("getExitsWithSelect: %s", err)
	}
	for i := range lobbyIDs {
		exits[i].Lobby, err = GetLobby(tx, lobbyIDs[i])
		if err != nil {
			return exits, fmt.Errorf("getExitsWithSelect: %s", err)
		}
	}
	return exits, nil
}

// GetExit returns the Exit with the given ID
func GetExit(node sqalx.Node, id int) (*Exit, error) {
	s := sdb.Select().
		Where(sq.Eq{"id": id})
	exits, err := getExitsWithSelect(node, s)
	if err != nil {
		return nil, err
	}
	if len(exits) == 0 {
		return nil, errors.New("Exit not found")
	}
	return exits[0], nil
}

// Add adds the exit
func (exit *Exit) Add(node sqalx.Node) error {
	tx, err := node.Beginx()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	err = exit.Lobby.Update(tx)
	if err != nil {
		return errors.New("AddExit: " + err.Error())
	}

	_, err = sdb.Insert("station_lobby_exit").
		Columns("lobby_id", "world_coord", "streets", "type").
		Values(exit.Lobby.ID, exit.WorldCoord, exit.Streets, exit.Type).
		RunWith(tx).Exec()

	if err != nil {
		return errors.New("AddExit: " + err.Error())
	}
	return tx.Commit()
}

// Delete deletes the exit
func (exit *Exit) Delete(node sqalx.Node) error {
	tx, err := node.Beginx()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	_, err = sdb.Delete("station_lobby_exit").
		Where(sq.Eq{"id": exit.ID}).RunWith(tx).Exec()
	if err != nil {
		return fmt.Errorf("RemoveExit: %s", err)
	}
	return tx.Commit()
}
