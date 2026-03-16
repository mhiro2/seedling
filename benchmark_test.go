package seedling_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/mhiro2/seedling"
	"github.com/mhiro2/seedling/seedlingtest"
)

func BenchmarkInsertOne(b *testing.B) {
	reg := seedlingtest.NewRegistry()
	seedlingtest.RegisterBasic(b, reg, seedlingtest.DefaultBasicInserters(seedlingtest.NewIDSequence()))

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		seedling.NewSession[Company](reg).InsertOne(b, nil)
	}
}

func BenchmarkInsertMany(b *testing.B) {
	reg := seedlingtest.NewRegistry()
	seedlingtest.RegisterBasic(b, reg, seedlingtest.DefaultBasicInserters(seedlingtest.NewIDSequence()))

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		seedling.NewSession[Company](reg).InsertMany(b, nil, 100)
	}
}

func BenchmarkBuild(b *testing.B) {
	reg := seedlingtest.NewRegistry()
	seedlingtest.RegisterBasic(b, reg, seedlingtest.DefaultBasicInserters(seedlingtest.NewIDSequence()))

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		seedling.NewSession[Task](reg).Build(b)
	}
}

func BenchmarkInsertOne_WithDependencies(b *testing.B) {
	reg := seedlingtest.NewRegistry()
	seedlingtest.RegisterBasic(b, reg, seedlingtest.DefaultBasicInserters(seedlingtest.NewIDSequence()))

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		seedling.NewSession[Task](reg).InsertOne(b, nil)
	}
}

func BenchmarkInsertMany_LargeBatch(b *testing.B) {
	reg := seedlingtest.NewRegistry()
	seedlingtest.RegisterBasic(b, reg, seedlingtest.DefaultBasicInserters(seedlingtest.NewIDSequence()))

	for _, n := range []int{100, 500, 1000} {
		b.Run(fmt.Sprintf("n=%d", n), func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				seedling.NewSession[Company](reg).InsertMany(b, nil, n)
			}
		})
	}
}

func BenchmarkInsertMany_WithDependencies_LargeBatch(b *testing.B) {
	reg := seedlingtest.NewRegistry()
	seedlingtest.RegisterBasic(b, reg, seedlingtest.DefaultBasicInserters(seedlingtest.NewIDSequence()))

	for _, n := range []int{100, 500, 1000} {
		b.Run(fmt.Sprintf("n=%d", n), func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				seedling.NewSession[Task](reg).InsertMany(b, nil, n)
			}
		})
	}
}

// Deep dependency chain models (D1 -> D2 -> ... -> D12)

type D1 struct {
	ID   int
	Name string
}
type D2 struct {
	ID   int
	D1ID int
	Name string
}
type D3 struct {
	ID   int
	D2ID int
	Name string
}
type D4 struct {
	ID   int
	D3ID int
	Name string
}
type D5 struct {
	ID   int
	D4ID int
	Name string
}
type D6 struct {
	ID   int
	D5ID int
	Name string
}
type D7 struct {
	ID   int
	D6ID int
	Name string
}
type D8 struct {
	ID   int
	D7ID int
	Name string
}
type D9 struct {
	ID   int
	D8ID int
	Name string
}
type D10 struct {
	ID   int
	D9ID int
	Name string
}
type D11 struct {
	ID    int
	D10ID int
	Name  string
}
type D12 struct {
	ID    int
	D11ID int
	Name  string
}

func registerDeepChain(b *testing.B, reg *seedling.Registry, ids *seedlingtest.IDSequence) {
	b.Helper()

	seedling.MustRegisterTo(reg, seedling.Blueprint[D1]{
		Name: "d1", Table: "d1s", PKField: "ID",
		Defaults: func() D1 { return D1{Name: "d1"} },
		Insert: func(_ context.Context, _ seedling.DBTX, v D1) (D1, error) {
			v.ID = ids.Next()
			return v, nil
		},
	})
	seedling.MustRegisterTo(reg, seedling.Blueprint[D2]{
		Name: "d2", Table: "d2s", PKField: "ID",
		Defaults: func() D2 { return D2{Name: "d2"} },
		Relations: []seedling.Relation{
			{Name: "d1", Kind: seedling.BelongsTo, LocalField: "D1ID", RefBlueprint: "d1"},
		},
		Insert: func(_ context.Context, _ seedling.DBTX, v D2) (D2, error) {
			v.ID = ids.Next()
			return v, nil
		},
	})
	seedling.MustRegisterTo(reg, seedling.Blueprint[D3]{
		Name: "d3", Table: "d3s", PKField: "ID",
		Defaults: func() D3 { return D3{Name: "d3"} },
		Relations: []seedling.Relation{
			{Name: "d2", Kind: seedling.BelongsTo, LocalField: "D2ID", RefBlueprint: "d2"},
		},
		Insert: func(_ context.Context, _ seedling.DBTX, v D3) (D3, error) {
			v.ID = ids.Next()
			return v, nil
		},
	})
	seedling.MustRegisterTo(reg, seedling.Blueprint[D4]{
		Name: "d4", Table: "d4s", PKField: "ID",
		Defaults: func() D4 { return D4{Name: "d4"} },
		Relations: []seedling.Relation{
			{Name: "d3", Kind: seedling.BelongsTo, LocalField: "D3ID", RefBlueprint: "d3"},
		},
		Insert: func(_ context.Context, _ seedling.DBTX, v D4) (D4, error) {
			v.ID = ids.Next()
			return v, nil
		},
	})
	seedling.MustRegisterTo(reg, seedling.Blueprint[D5]{
		Name: "d5", Table: "d5s", PKField: "ID",
		Defaults: func() D5 { return D5{Name: "d5"} },
		Relations: []seedling.Relation{
			{Name: "d4", Kind: seedling.BelongsTo, LocalField: "D4ID", RefBlueprint: "d4"},
		},
		Insert: func(_ context.Context, _ seedling.DBTX, v D5) (D5, error) {
			v.ID = ids.Next()
			return v, nil
		},
	})
	seedling.MustRegisterTo(reg, seedling.Blueprint[D6]{
		Name: "d6", Table: "d6s", PKField: "ID",
		Defaults: func() D6 { return D6{Name: "d6"} },
		Relations: []seedling.Relation{
			{Name: "d5", Kind: seedling.BelongsTo, LocalField: "D5ID", RefBlueprint: "d5"},
		},
		Insert: func(_ context.Context, _ seedling.DBTX, v D6) (D6, error) {
			v.ID = ids.Next()
			return v, nil
		},
	})
	seedling.MustRegisterTo(reg, seedling.Blueprint[D7]{
		Name: "d7", Table: "d7s", PKField: "ID",
		Defaults: func() D7 { return D7{Name: "d7"} },
		Relations: []seedling.Relation{
			{Name: "d6", Kind: seedling.BelongsTo, LocalField: "D6ID", RefBlueprint: "d6"},
		},
		Insert: func(_ context.Context, _ seedling.DBTX, v D7) (D7, error) {
			v.ID = ids.Next()
			return v, nil
		},
	})
	seedling.MustRegisterTo(reg, seedling.Blueprint[D8]{
		Name: "d8", Table: "d8s", PKField: "ID",
		Defaults: func() D8 { return D8{Name: "d8"} },
		Relations: []seedling.Relation{
			{Name: "d7", Kind: seedling.BelongsTo, LocalField: "D7ID", RefBlueprint: "d7"},
		},
		Insert: func(_ context.Context, _ seedling.DBTX, v D8) (D8, error) {
			v.ID = ids.Next()
			return v, nil
		},
	})
	seedling.MustRegisterTo(reg, seedling.Blueprint[D9]{
		Name: "d9", Table: "d9s", PKField: "ID",
		Defaults: func() D9 { return D9{Name: "d9"} },
		Relations: []seedling.Relation{
			{Name: "d8", Kind: seedling.BelongsTo, LocalField: "D8ID", RefBlueprint: "d8"},
		},
		Insert: func(_ context.Context, _ seedling.DBTX, v D9) (D9, error) {
			v.ID = ids.Next()
			return v, nil
		},
	})
	seedling.MustRegisterTo(reg, seedling.Blueprint[D10]{
		Name: "d10", Table: "d10s", PKField: "ID",
		Defaults: func() D10 { return D10{Name: "d10"} },
		Relations: []seedling.Relation{
			{Name: "d9", Kind: seedling.BelongsTo, LocalField: "D9ID", RefBlueprint: "d9"},
		},
		Insert: func(_ context.Context, _ seedling.DBTX, v D10) (D10, error) {
			v.ID = ids.Next()
			return v, nil
		},
	})
	seedling.MustRegisterTo(reg, seedling.Blueprint[D11]{
		Name: "d11", Table: "d11s", PKField: "ID",
		Defaults: func() D11 { return D11{Name: "d11"} },
		Relations: []seedling.Relation{
			{Name: "d10", Kind: seedling.BelongsTo, LocalField: "D10ID", RefBlueprint: "d10"},
		},
		Insert: func(_ context.Context, _ seedling.DBTX, v D11) (D11, error) {
			v.ID = ids.Next()
			return v, nil
		},
	})
	seedling.MustRegisterTo(reg, seedling.Blueprint[D12]{
		Name: "d12", Table: "d12s", PKField: "ID",
		Defaults: func() D12 { return D12{Name: "d12"} },
		Relations: []seedling.Relation{
			{Name: "d11", Kind: seedling.BelongsTo, LocalField: "D11ID", RefBlueprint: "d11"},
		},
		Insert: func(_ context.Context, _ seedling.DBTX, v D12) (D12, error) {
			v.ID = ids.Next()
			return v, nil
		},
	})
}

func BenchmarkInsertOne_DeepChain(b *testing.B) {
	ids := seedlingtest.NewIDSequence()
	reg := seedlingtest.NewRegistry()
	registerDeepChain(b, reg, ids)

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		seedling.NewSession[D12](reg).InsertOne(b, nil)
	}
}

func BenchmarkBuild_DeepChain(b *testing.B) {
	ids := seedlingtest.NewIDSequence()
	reg := seedlingtest.NewRegistry()
	registerDeepChain(b, reg, ids)

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		seedling.NewSession[D12](reg).Build(b)
	}
}
