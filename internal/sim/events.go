package sim

// nextRand advances a splitmix64 state and returns the new state plus a
// uniform float64 in [0,1). All event randomness flows through this so the
// sim stays deterministic: same GameState → same rolls.
func nextRand(state uint64) (uint64, float64) {
	state += 0x9E3779B97F4A7C15
	z := state
	z = (z ^ (z >> 30)) * 0xBF58476D1CE4E5B9
	z = (z ^ (z >> 27)) * 0x94D049BB133111EB
	z ^= z >> 31
	return state, float64(z>>11) / float64(1<<53)
}
