package dataobjects

import (
	"errors"
	"fmt"
	"net"
	"time"

	sq "github.com/gbl08ma/squirrel"
	"github.com/gbl08ma/sqalx"
)

// AndroidPairRequest contains info for a pair request issued by an Android app
type AndroidPairRequest struct {
	Nonce       string
	RequestTime time.Time
	AndroidID   string
	IPaddress   net.IP
}

// getAndroidPairRequestsWithSelect returns a slice with all AndroidPairRequest that match the conditions in sbuilder
func getAndroidPairRequestsWithSelect(node sqalx.Node, sbuilder sq.SelectBuilder) ([]*AndroidPairRequest, error) {
	requests := []*AndroidPairRequest{}

	tx, err := node.Beginx()
	if err != nil {
		return requests, err
	}
	defer tx.Commit() // read-only tx

	rows, err := sbuilder.Columns("nonce", "request_time", "android_id", "ip_address").
		From("android_pair_request").
		RunWith(tx).Query()
	if err != nil {
		return requests, fmt.Errorf("getAndroidPairRequestsWithSelect: %s", err)
	}

	defer rows.Close()

	for rows.Next() {
		var request AndroidPairRequest
		var ipAddr string
		err := rows.Scan(
			&request.Nonce,
			&request.RequestTime,
			&request.AndroidID,
			&ipAddr)
		if err != nil {
			return requests, fmt.Errorf("getAndroidPairRequestsWithSelect: %s", err)
		}

		request.IPaddress = net.ParseIP(ipAddr)
		if request.IPaddress == nil {
			return requests, fmt.Errorf("getAndroidPairRequestsWithSelect: invalid IP address")
		}

		requests = append(requests, &request)
	}
	if err := rows.Err(); err != nil {
		return requests, fmt.Errorf("getAndroidPairRequestsWithSelect: %s", err)
	}
	return requests, nil
}

// NewAndroidPairRequest creates a new AndroidPairRequest and returns it
// Does NOT store the request in the DB
func NewAndroidPairRequest(nonce string, androidID string, ipAddress net.IP) *AndroidPairRequest {
	return &AndroidPairRequest{
		Nonce:       nonce,
		RequestTime: time.Now().UTC(),
		AndroidID:   androidID,
		IPaddress:   ipAddress,
	}
}

// Store stores this request in the DB
// To be used if the request is successful (i.e. the client was sent a API pair)
func (request *AndroidPairRequest) Store(node sqalx.Node) error {
	tx, err := node.Beginx()
	if err != nil {
		return errors.New("Store: " + err.Error())
	}
	defer tx.Rollback()

	_, err = sdb.Insert("android_pair_request").
		Columns("nonce", "request_time", "android_id", "ip_address").
		Values(request.Nonce,
			request.RequestTime,
			request.AndroidID,
			request.IPaddress.String()).
		RunWith(tx).Exec()

	if err != nil {
		return errors.New("Store: " + err.Error())
	}
	err = tx.Commit()
	if err != nil {
		return errors.New("Store: " + err.Error())
	}
	return nil
}

// CalculateActivationTime returns the recommended activation time for a API pair
// issued in response to this request
// If the returned time is zero, a API pair should not be granted
func (request *AndroidPairRequest) CalculateActivationTime(node sqalx.Node, maxTimestampSkew time.Duration) (time.Time, error) {
	tx, err := node.Beginx()
	if err != nil {
		return time.Time{}, err
	}
	defer tx.Commit() // read-only tx

	s := sdb.Select().
		Where(sq.Or{
			sq.Eq{"nonce": request.Nonce},
			sq.Eq{"android_id": request.AndroidID},
			sq.Eq{"ip_address": request.IPaddress.String()},
		})

	requests, err := getAndroidPairRequestsWithSelect(tx, s)
	if err != nil {
		return time.Time{}, err
	}

	activation := time.Now().UTC()
	for _, pastRequest := range requests {
		// let's find out in which way this one is bad
		diff := request.RequestTime.Sub(pastRequest.RequestTime)
		diff = maxDuration(diff, -diff)

		if request.Nonce == pastRequest.Nonce {
			if diff < maxTimestampSkew {
				// nope, this nonce was used too recently
				return time.Time{}, nil
			}
		}

		if request.AndroidID == pastRequest.AndroidID {
			switch {
			case diff < 5*time.Minute:
				activation = activation.Add(2 * time.Hour)
			case diff < 30*time.Minute:
				activation = activation.Add(1 * time.Hour)
			case diff < 2*time.Hour:
				activation = activation.Add(30 * time.Minute)
			case diff < 24*time.Hour:
				activation = activation.Add(10 * time.Minute)
			default:
				// probably an honest reinstall. Penalize just a bit
				activation = activation.Add(1 * time.Minute)
			}
		}

		if request.IPaddress.Equal(pastRequest.IPaddress) {
			switch {
			case diff < 5*time.Minute:
				activation = activation.Add(1 * time.Hour)
			case diff < 30*time.Minute:
				activation = activation.Add(30 * time.Minute)
			case diff < 2*time.Hour:
				activation = activation.Add(15 * time.Minute)
			case diff < 24*time.Hour:
				activation = activation.Add(5 * time.Minute)
			default:
				// probably just two people installing from the same location
				// or a user with two devices
				activation = activation.Add(1 * time.Minute)
			}
		}
	}
	if activation.Sub(time.Now().UTC()) > 24*time.Hour {
		// don't make anyone wait more than 24 hours

		// TODO FIXME this makes the whole thing mostly pointless
		// an attacker can just "spawn" a thousand devices
		// (and they can even all have the same IP address and Android ID)
		// despite the fact that the activation time should then be extremely high,
		// since we're setting this upper bound, the attacker can just wait 24 hours
		// and by then, all of the attacker's pairs will be activated
		// without this upper bound, if someone requests a thousand pairings from the same IP
		// they'll have to wait 1000 hours

		// situations where IP addresses might not be so unique:
		// - people using the free Go Wifi at subway stations (whoops)

		// conclusion:
		// remove this check once we're sure IP addresses and Android IDs are unique enough
		activation = time.Now().UTC().Add(24 * time.Hour)
	}
	return activation, nil
}

func maxDuration(d1 time.Duration, d2 time.Duration) time.Duration {
	if d1 > d2 {
		return d1
	}
	return d2
}
