package repo

// MergeSquadStateIntoRepoMap reads the live claude-squad state and adds any
// worktree -> repo pairs not already in the durable repo-map index, then
// writes the index back. It returns the number of entries added.
//
// sweep calls this so worktrees that were alive at sweep time get a durable
// mapping even if their SessionStart hook never fired. Existing repo-map
// entries are authoritative and never overwritten: the hook captured those
// deliberately, and claude-squad state is the lower-confidence source.
func MergeSquadStateIntoRepoMap(squadStatePath, repoMapPath string) (int, error) {
	squadIdx, err := LoadSquadState(squadStatePath)
	if err != nil {
		return 0, err
	}
	repoMap, err := LoadRepoMap(repoMapPath)
	if err != nil {
		return 0, err
	}

	added := 0
	for worktree, origin := range squadIdx {
		if _, exists := repoMap[worktree]; exists {
			continue
		}
		repoMap[worktree] = origin
		added++
	}

	if added == 0 {
		return 0, nil
	}
	if err := WriteRepoMap(repoMapPath, repoMap); err != nil {
		return 0, err
	}
	return added, nil
}
