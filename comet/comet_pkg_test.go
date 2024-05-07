package comet

import (
	"testing"

	"github.com/stretchr/testify/require"
)

type paginateTest struct {
	page,
	requestedPageSize,
	totalResults,
	expectedSkip,
	expectedPageSize int
}

func TestPaginateSuccess(t *testing.T) {
	tests := map[string]paginateTest{
		"no results": {
			page:              1,
			requestedPageSize: 10,
			totalResults:      0,
			expectedSkip:      0,
			expectedPageSize:  0,
		},
		"one result": {
			page:              1,
			requestedPageSize: 10,
			totalResults:      1,
			expectedSkip:      0,
			expectedPageSize:  1,
		},
		"exactly one page": {
			page:              1,
			requestedPageSize: 10,
			totalResults:      10,
			expectedSkip:      0,
			expectedPageSize:  10,
		},
		"one  non-returned result": {
			page:              1,
			requestedPageSize: 10,
			totalResults:      11,
			expectedSkip:      0,
			expectedPageSize:  10,
		},
		"return single second-page result": {
			page:              2,
			requestedPageSize: 10,
			totalResults:      11,
			expectedSkip:      10,
			expectedPageSize:  1,
		},
		"return 10 second-page results": {
			page:              2,
			requestedPageSize: 10,
			totalResults:      20,
			expectedSkip:      10,
			expectedPageSize:  10,
		},
		"return intermediate page": {
			page:              16,
			requestedPageSize: 10,
			totalResults:      200,
			expectedSkip:      150,
			expectedPageSize:  10,
		},
	}
	for description, test := range tests {
		t.Run(description, func(t *testing.T) {
			skip, pageSize, err := paginate(&test.page, &test.requestedPageSize, test.totalResults)
			require.Equal(t, test.expectedSkip, skip)
			require.Equal(t, test.expectedPageSize, pageSize)
			require.NoError(t, err)
		})
	}
}

func TestPaginateFailure(t *testing.T) {
	tests := map[string]paginateTest{
		"pagenumber = 0": {
			page:              0,
			requestedPageSize: 10,
			totalResults:      0,
		},
		"pagenumber < 0": {
			page:              -1,
			requestedPageSize: 10,
			totalResults:      100,
		},
	}
	for description, test := range tests {
		t.Run(description, func(t *testing.T) {
			_, _, err := paginate(&test.page, &test.requestedPageSize, test.totalResults)
			require.Error(t, err)
		})
	}
}
