package ui

import "github.com/gdamore/tcell/v2"

// These are the standard 16-color VGA values, kept explicit so true-color
// terminals do not substitute their own indexed-color palette.
var (
	turboBlue      = tcell.NewHexColor(0x0000AA)
	turboYellow    = tcell.NewHexColor(0xFFFF55)
	turboWhite     = tcell.NewHexColor(0xFFFFFF)
	turboLightGray = tcell.NewHexColor(0xAAAAAA)
	turboBlack     = tcell.NewHexColor(0x000000)
	turboRed       = tcell.NewHexColor(0xAA0000)
	turboGreen     = tcell.NewHexColor(0x00AA00)
	turboLightCyan = tcell.NewHexColor(0x55FFFF)
)
