package ui

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/hangxie/rapidgo/internal/editor"
)

func TestSyntaxCacheFollowsBufferRevisionAndIdentity(t *testing.T) {
	t.Parallel()
	buffer, err := editor.New("var x = 1")
	require.NoError(t, err)
	cache := &syntaxCache{}
	first := cache.spansFor(buffer)
	require.NotEmpty(t, first)
	assert.Equal(t, &first[0], &cache.spansFor(buffer)[0])

	buffer.MoveEnd(false)
	require.NoError(t, buffer.Insert(" // note"))
	second := cache.spansFor(buffer)
	assert.Len(t, second, len(first)+1)

	other, err := editor.New("package p")
	require.NoError(t, err)
	third := cache.spansFor(other)
	require.Len(t, third, 1)
	assert.Equal(t, other, cache.buffer)
}
