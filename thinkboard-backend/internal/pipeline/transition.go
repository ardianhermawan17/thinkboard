package pipeline

// Allowed reports whether to is a valid forward transition from from — i.e.
// to is the immediate successor of from in Order. Pure, no I/O; derives
// entirely from Order so it never duplicates the stage sequence.
func Allowed(from, to Stage) bool {
	i := indexOf(from)
	j := indexOf(to)
	if i < 0 || j < 0 {
		return false
	}
	return j == i+1
}

func indexOf(s Stage) int {
	for i, o := range Order {
		if o == s {
			return i
		}
	}
	return -1
}
