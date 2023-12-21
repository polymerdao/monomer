package peptide

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPaginator_Paginate(t *testing.T) {
	p := NewPaginater(30)

	// Test case 1: pagePtr and perPagePtr are nil
	skipCount, pageSize, err := p.Paginate(nil, nil, 100)
	require.NoError(t, err)
	require.Equal(t, 0, skipCount)
	require.Equal(t, 30, pageSize)

	// Test case 2: pagePtr is nil, perPagePtr is non-nil
	skipCount, pageSize, err = p.Paginate(nil, intPtr(50), 100)
	require.NoError(t, err)
	require.Equal(t, 0, skipCount)
	require.Equal(t, 50, pageSize)

	// Test case 3: pagePtr is non-nil, perPagePtr is nil
	skipCount, pageSize, err = p.Paginate(intPtr(2), nil, 100)
	require.NoError(t, err)
	require.Equal(t, 30, skipCount)
	require.Equal(t, 30, pageSize)

	// Test case 4: pagePtr and perPagePtr are non-nil
	skipCount, pageSize, err = p.Paginate(intPtr(2), intPtr(50), 100)
	require.NoError(t, err)
	require.Equal(t, 50, skipCount)
	require.Equal(t, 50, pageSize)

	// Test case 5: pagePtr is out of range
	_, _, err = p.Paginate(intPtr(5), nil, 100)
	require.Error(t, err)
	expectedErr := fmt.Errorf("page must be between 1 and 4, but got 5; totalCount:100, pageSize: 30")
	require.EqualError(t, err, expectedErr.Error())

	// Test case 6: totalCount is less than perPage
	skipCount, pageSize, err = p.Paginate(intPtr(1), intPtr(50), 20)
	require.NoError(t, err)
	require.Equal(t, 0, skipCount)
	require.Equal(t, 20, pageSize)

	// Test case 7: totalCount is equal to perPage
	skipCount, pageSize, err = p.Paginate(intPtr(1), intPtr(50), 50)
	require.NoError(t, err)
	require.Equal(t, 0, skipCount)
	require.Equal(t, 50, pageSize)

	// Test case 8: totalCount is greater than perPage
	skipCount, pageSize, err = p.Paginate(intPtr(2), intPtr(50), 100)
	require.NoError(t, err)
	require.Equal(t, 50, skipCount)
	require.Equal(t, 50, pageSize)

	skipCount, pageSize, err = p.Paginate(intPtr(1), intPtr(3), 7)
	require.NoError(t, err)
	require.Equal(t, 0, skipCount)
	require.Equal(t, 3, pageSize)

	skipCount, pageSize, err = p.Paginate(intPtr(2), intPtr(3), 7)
	require.NoError(t, err)
	require.Equal(t, 3, skipCount)
	require.Equal(t, 3, pageSize)

	skipCount, pageSize, err = p.Paginate(intPtr(3), intPtr(3), 7)
	require.NoError(t, err)
	require.Equal(t, 6, skipCount)
	require.Equal(t, 1, pageSize)
}

func intPtr(i int) *int {
	return &i
}
