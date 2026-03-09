package dissect

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/rs/zerolog/log"
)

func winningTeamFromScoreDelta(teams [2]Team) (int, bool) {
	delta0 := teams[0].Score - teams[0].StartingScore
	delta1 := teams[1].Score - teams[1].StartingScore
	switch {
	case delta0 == 1 && delta1 == 0:
		return 0, true
	case delta0 == 0 && delta1 == 1:
		return 1, true
	default:
		return -1, false
	}
}

func readTime(r *Reader) error {
	time, err := r.Uint32()
	if err != nil {
		return err
	}
	r.time = float64(time)
	r.timeRaw = fmt.Sprintf("%d:%02d", time/60, time%60)
	return nil
}

func readY7Time(r *Reader) error {
	time, err := r.String()
	parts := strings.Split(time, ":")
	if len(parts) == 1 {
		seconds, err := strconv.ParseFloat(parts[0], 64)
		if err != nil {
			return err
		}
		r.time = seconds
		r.timeRaw = parts[0]
		return nil
	}
	minutes, err := strconv.Atoi(parts[0])
	if err != nil {
		return err
	}
	seconds, err := strconv.Atoi(parts[1])
	if err != nil {
		return err
	}
	r.time = float64((minutes * 60) + seconds)
	r.timeRaw = time
	return nil
}

func (r *Reader) roundEnd() {
	log.Debug().Msg("round_end")

	planter := -1
	disabler := -1
	deaths := make(map[int]int)
	sizes := make(map[int]int)
	roles := make(map[int]TeamRole)

	for _, p := range r.Header.Players {
		sizes[p.TeamIndex] += 1
		roles[p.TeamIndex] = r.Header.Teams[p.TeamIndex].Role
	}

	if r.Header.CodeVersion >= Y9S4 {
		for i := range r.Header.Teams {
			r.Header.Teams[i].Won = false
			r.Header.Teams[i].WinCondition = ""
		}
		if winner, ok := winningTeamFromScoreDelta(r.Header.Teams); ok {
			r.Header.Teams[winner].Won = true
		}
	}

	for _, u := range r.MatchFeedback {
		switch u.Type {
		case Kill:
			i := r.Header.Players[r.PlayerIndexByUsername(u.Target)].TeamIndex
			deaths[i] = deaths[i] + 1
			if len(u.usernameFromScoreboard) > 0 {
				u.Username = u.usernameFromScoreboard
			}
		case Death:
			i := r.Header.Players[r.PlayerIndexByUsername(u.Username)].TeamIndex
			deaths[i] = deaths[i] + 1
		case DefuserPlantComplete:
			planter = r.PlayerIndexByUsername(u.Username)
		case DefuserDisableComplete:
			disabler = r.PlayerIndexByUsername(u.Username)
		}
	}

	if r.Header.CodeVersion >= Y9S4 {
		for i, team := range r.Header.Teams {
			if !team.Won {
				continue
			}
			if planter > -1 && r.Header.Players[planter].TeamIndex == i && roles[i] == Attack {
				r.Header.Teams[i].WinCondition = DefusedBomb
				return
			}
			if disabler > -1 && r.Header.Players[disabler].TeamIndex == i && roles[i] == Defense {
				r.Header.Teams[i].WinCondition = DisabledDefuser
				return
			}
		}
		return
	}

	if disabler > -1 {
		i := r.Header.Players[disabler].TeamIndex
		r.Header.Teams[i].Won = true
		r.Header.Teams[i].WinCondition = DisabledDefuser
		return
	}

	if planter > -1 {
		r.Header.Teams[r.Header.Players[planter].TeamIndex].Won = true
		r.Header.Teams[r.Header.Players[planter].TeamIndex].WinCondition = DefusedBomb
		return
	}

	if deaths[0] == sizes[0] {
		if planter > -1 && roles[0] == Attack {
			return
		}
		r.Header.Teams[1].Won = true
		r.Header.Teams[1].WinCondition = KilledOpponents
		return
	}
	if deaths[1] == sizes[1] {
		if planter > -1 && roles[1] == Attack {
			return
		}
		r.Header.Teams[0].Won = true
		r.Header.Teams[0].WinCondition = KilledOpponents
		return
	}

	i := 0
	if roles[1] == Defense {
		i = 1
	}

	r.Header.Teams[i].Won = true
	r.Header.Teams[i].WinCondition = Time
}
