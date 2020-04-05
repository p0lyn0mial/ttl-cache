package eviction_store

import (
	"fmt"
	"testing"
	"time"

	"k8s.io/apimachinery/pkg/util/clock"
	"k8s.io/apimachinery/pkg/util/sets"
)

type itemTest struct {
	key string
}

func TestEvictionStore(t *testing.T) {
	type step struct {
		clockStep time.Duration
		keysToGet sets.String
	}

	defaultKeyFunc := func(obj interface{}) string {
		return obj.(*itemTest).key
	}

	scenarios := []struct {
		name         string
		objs         []*itemTest
		steps        []step
		expectedObjs func([]*itemTest) []*itemTest
	}{
		{
			name: "none expired",
			objs: []*itemTest{{key: "1"}, {key: "2"}, {key: "3"}},
			steps: []step{
				{clockStep: 9 * time.Minute, keysToGet: sets.NewString("1", "2", "3")},
			},
			expectedObjs: func(allObjs []*itemTest) []*itemTest {
				return filterObjs(allObjs, sets.NewString("1", "2", "3"))
			},
		},

		{
			name: "all expired",
			objs: []*itemTest{{key: "1"}, {key: "2"}, {key: "3"}},
			steps: []step{
				{clockStep: 11 * time.Minute},
			},
			expectedObjs: func(allObjs []*itemTest) []*itemTest {
				return []*itemTest{}
			},
		},

		{
			name: "key 2 expired",
			objs: []*itemTest{{key: "1"}, {key: "2"}, {key: "3"}},
			steps: []step{
				{clockStep: 5 * time.Minute, keysToGet: sets.NewString("1", "3")},
				{clockStep: 6 * time.Minute, keysToGet: sets.NewString("1", "3")},
			},
			expectedObjs: func(allObjs []*itemTest) []*itemTest {
				return filterObjs(allObjs, sets.NewString("1", "3"))
			},
		},

		{
			name: "key 2 expired - may steps",
			objs: []*itemTest{{key: "1"}, {key: "2"}, {key: "3"}},
			steps: []step{
				{clockStep: 8 * time.Minute, keysToGet: sets.NewString("1", "3")},
				{clockStep: 8 * time.Minute, keysToGet: sets.NewString("1", "3")},
				{clockStep: 8 * time.Minute, keysToGet: sets.NewString("1", "3")},
				{clockStep: 8 * time.Minute, keysToGet: sets.NewString("1", "3")},
				{clockStep: 8 * time.Minute, keysToGet: sets.NewString("1", "3")},
			},
			expectedObjs: func(allObjs []*itemTest) []*itemTest {
				return filterObjs(allObjs, sets.NewString("1", "3"))
			},
		},
		{
			name: "key 2 expired - get after expiration",
			objs: []*itemTest{{key: "1"}, {key: "2"}, {key: "3"}},
			steps: []step{
				{clockStep: 8 * time.Minute, keysToGet: sets.NewString("1", "3")},
				{clockStep: 8 * time.Minute, keysToGet: sets.NewString("1", "2", "3")},
				{clockStep: 8 * time.Minute, keysToGet: sets.NewString("1", "3")},
			},
			expectedObjs: func(allObjs []*itemTest) []*itemTest {
				return filterObjs(allObjs, sets.NewString("1", "3"))
			},
		},

		{
			name: "single item - expiration",
			objs: []*itemTest{{key: "1"}},
			steps: []step{
				{clockStep: 11 * time.Minute, keysToGet: sets.NewString("1")},
			},
			expectedObjs: func(allObjs []*itemTest) []*itemTest {
				return []*itemTest{}
			},
		},

		{
			name: "single item - no expiration",
			objs: []*itemTest{{key: "1"}},
			steps: []step{
				{clockStep: 8 * time.Minute, keysToGet: sets.NewString("1")},
			},
			expectedObjs: func(allObjs []*itemTest) []*itemTest {
				return filterObjs(allObjs, sets.NewString("1"))
			},
		},

		{
			name: "empty store",
			objs: []*itemTest{{}},
			steps: []step{
				{clockStep: 8 * time.Minute, keysToGet: sets.NewString("1")},
			},
			expectedObjs: func(allObjs []*itemTest) []*itemTest {
				return []*itemTest{}
			},
		},
	}
	for _, scenario := range scenarios {
		t.Run(scenario.name, func(t *testing.T) {
			// test data
			fakeClock := &clock.FakeClock{}

			// act
			target := New(defaultKeyFunc, 10*time.Minute, fakeClock)
			for _, obj := range scenario.objs {
				target.Add(obj)
			}

			for _, step := range scenario.steps {
				fakeClock.Step(step.clockStep)
				for _, objToGet := range step.keysToGet.List() {
					target.Get(objToGet)
				}
			}

			// validate
			expectedObjs := scenario.expectedObjs(scenario.objs)
			for _, obj := range scenario.objs {
				actualObj := target.Get(defaultKeyFunc(obj))
				found := false
				for _, expectedObj := range expectedObjs {
					if actualObj.(*itemTest) == expectedObj {
						fmt.Printf("actual %p, expected = %p\n", actualObj.(*itemTest), expectedObj)
						found = true
						break
					}
				}
				if found {
					return
				}
				okToMiss := shouldMiss(scenario.objs, expectedObjs, defaultKeyFunc(obj), defaultKeyFunc)
				if !found && okToMiss {
					return
				}
				t.Fatalf("an object with key %s not found", defaultKeyFunc(obj))
			}
		})
	}
}

func filterObjs(objs []*itemTest, interestingKeys sets.String) []*itemTest {
	ret := []*itemTest{}
	for _, obj := range objs {
		if interestingKeys.Has(obj.key) {
			ret = append(ret, obj)
		}
	}
	return ret
}

func shouldMiss(actual []*itemTest, expected []*itemTest, missingKey string, keyFunc func(obj interface{}) string) bool {
	actualKeySet := sets.NewString()
	for _, a := range actual {
		actualKeySet.Insert(keyFunc(a))
	}

	expectedKeySet := sets.NewString()
	for _, e := range expected {
		expectedKeySet.Insert(keyFunc(e))
	}

	return actualKeySet.Difference(expectedKeySet).Has(missingKey)
}
