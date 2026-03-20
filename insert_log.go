package seedling

import "github.com/mhiro2/seedling/internal/executor"

func toExecutorLogFn(fn func(InsertLog)) func(executor.LogEntry) {
	if fn == nil {
		return nil
	}

	return func(entry executor.LogEntry) {
		fn(toInsertLog(entry))
	}
}

func toInsertLog(entry executor.LogEntry) InsertLog {
	return InsertLog{
		Step:       entry.Step,
		Blueprint:  entry.Blueprint,
		Table:      entry.Table,
		Provided:   entry.Provided,
		FKBindings: toFKBindings(entry.FKBindings),
	}
}

func toFKBindings(bindings []executor.FKBinding) []FKBinding {
	out := make([]FKBinding, len(bindings))
	for i, binding := range bindings {
		out[i] = FKBinding{
			ChildField:      binding.ChildField,
			ParentBlueprint: binding.ParentBlueprint,
			ParentTable:     binding.ParentTable,
			ParentField:     binding.ParentField,
			Value:           binding.Value,
		}
	}
	return out
}
