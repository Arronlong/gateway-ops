package monitor

import "context"

func (walker *usageHistoryWalker) readCounts(ctx context.Context, window usageHistoryWindow, level string) (usageWindowCounts, error) {
	if walker.Progress != nil {
		walker.Progress(UsageWindowIntegrity{AccountID: walker.AccountID, AccountName: walker.AccountName, Since: window.Since, Until: window.Until, Level: level, Status: "checking"})
	}
	counts, err := walker.Count(ctx, window)
	if err == nil {
		walker.Report.CheckedWindows++
	}
	return counts, err
}

func (walker *usageHistoryWalker) walkIncrementalParts(ctx context.Context, window usageHistoryWindow, counts usageWindowCounts, parts []usageHistoryWindow, level string) error {
	remaining := counts
	for index, part := range parts {
		if err := ctx.Err(); err != nil {
			return err
		}
		child := remaining
		if index < len(parts)-1 {
			var err error
			child, err = walker.readCounts(ctx, part, level)
			if err != nil {
				return err
			}
		}
		rest := usageWindowCounts{Local: remaining.Local - child.Local, Remote: remaining.Remote - child.Remote}
		if rest.Remote < 0 {
			return usageCountConflict(level, remaining.Remote, child.Remote)
		}
		if child.Local != child.Remote && index < len(parts)-1 {
			var err error
			rest, err = walker.readCounts(ctx, usageHistoryWindow{Since: part.Until, Until: window.Until}, "remaining")
			if err != nil {
				return err
			}
			if child.Remote+rest.Remote != remaining.Remote {
				return usageCountConflict(level, remaining.Remote, child.Remote+rest.Remote)
			}
		}
		if _, err := walker.walk(ctx, part, level, &child); err != nil {
			return err
		}
		remaining = rest
	}
	return nil
}
