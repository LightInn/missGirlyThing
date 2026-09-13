package services

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/png"
)

// RenderPollChart génère un graphique PNG (barres horizontales) du résultat.
// 100% stdlib : fond sombre style Discord, barres colorées par rang,
// proportionnelles à la moyenne (0-10). Les noms restent dans l'embed
// (accents/français intacts) ; le PNG porte rangs, moyennes et Copeland.
func RenderPollChart(res *PollResult) ([]byte, error) {
	if res == nil || len(res.Options) == 0 {
		return nil, fmt.Errorf("rien à dessiner")
	}

	const (
		W       = 760
		pad     = 28
		headH   = 64
		rowH    = 62
		rowGap  = 14
		footH   = 44
		barH    = 26
		barX    = 150
		scale   = 3
	)
	n := len(res.Options)
	H := headH + n*(rowH+rowGap) + footH

	bg := color.RGBA{43, 45, 49, 255}
	track := color.RGBA{30, 31, 34, 255}
	white := color.RGBA{255, 255, 255, 255}
	muted := color.RGBA{148, 155, 164, 255}
	palette := []color.RGBA{
		{250, 205, 80, 255},  // or
		{190, 200, 215, 255}, // argent
		{224, 146, 90, 255},  // bronze
		{88, 101, 242, 255},  // blurple
		{87, 242, 135, 255},  // vert
		{235, 105, 175, 255}, // rose
		{255, 255, 255, 255},
		{255, 255, 255, 255},
	}

	img := image.NewRGBA(image.Rect(0, 0, W, H))
	fill(img, bg)

	title := fmt.Sprintf("CONDORCET  •  %d VOTES", res.TotalVoters)
	drawText(img, title, pad, 26, scale, muted)

	drawText(img, "MOYENNE /10  +  COPELAND", pad, 46, 2, muted)

	barMaxW := W - barX - pad - 110
	for i, o := range res.Options {
		y := headH + i*(rowH+rowGap)
		c := palette[i%len(palette)]
		if i < len(palette) {
			c = palette[i]
		}

		// Pastille couleur + rang
		fillRect(img, pad, y+8, 18, 18, c)
		drawText(img, fmt.Sprintf("#%d", o.Rank), pad+26, y+10, scale, white)

		// Piste + barre (moyenne / 10)
		trackY := y + 34
		fillRect(img, barX, trackY, barMaxW, barH, track)
		w := int((o.Average / 10.0) * float64(barMaxW))
		if w < 2 && o.Average > 0 {
			w = 2
		}
		fillRect(img, barX, trackY, w, barH, c)
		// Reflet simple : ligne claire en haut de barre
		if w > 0 {
			light := lighten(c)
			fillRect(img, barX, trackY, w, 4, light)
		}

		// Score à droite de la barre
		cop := fmt.Sprintf("+%d", o.Copeland)
		if o.Copeland < 0 {
			cop = fmt.Sprintf("%d", o.Copeland)
		}
		drawText(img, fmt.Sprintf("%.1f  %s", o.Average, cop), barX+barMaxW+10, trackY+4, scale, white)
	}

	drawText(img, "VAINQUEUR CONDORCET = BAT CHAQUE OPTION EN DUEL", pad, H-20, 2, muted)

	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func fill(img *image.RGBA, c color.RGBA) {
	b := img.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			img.SetRGBA(x, y, c)
		}
	}
}

func fillRect(img *image.RGBA, x, y, w, h int, c color.RGBA) {
	b := img.Bounds()
	for j := y; j < y+h; j++ {
		for i := x; i < x+w; i++ {
			if i >= b.Min.X && i < b.Max.X && j >= b.Min.Y && j < b.Max.Y {
				img.SetRGBA(i, j, c)
			}
		}
	}
}

func lighten(c color.RGBA) color.RGBA {
	add := func(v uint8) uint8 {
		n := int(v) + 35
		if n > 255 {
			return 255
		}
		return uint8(n)
	}
	return color.RGBA{add(c.R), add(c.G), add(c.B), 255}
}

// Police pixel 3x5 maison, charset volontairement réduit aux infos du graphique.
var pixFont = map[rune][]string{
	'0': {"###", "# #", "# #", "# #", "###"},
	'1': {" # ", "## ", " # ", " # ", "###"},
	'2': {"###", "  #", "###", "#  ", "###"},
	'3': {"###", "  #", " ##", "  #", "###"},
	'4': {"# #", "# #", "###", "  #", "  #"},
	'5': {"###", "#  ", "###", "  #", "###"},
	'6': {"###", "#  ", "###", "# #", "###"},
	'7': {"###", "  #", "  #", " # ", " # "},
	'8': {"###", "# #", "###", "# #", "###"},
	'9': {"###", "# #", "###", "  #", "###"},
	'.': {"   ", "   ", "   ", "   ", " # "},
	'+': {"   ", " # ", "###", " # ", "   "},
	'-': {"   ", "   ", "###", "   ", "   "},
	'/': {"  #", "  #", " # ", "#  ", "#  "},
	'#': {"# #", "###", "# #", "###", "# #"},
	' ': {"   ", "   ", "   ", "   ", "   "},
	'V': {"# #", "# #", "# #", "# #", " # "},
	'O': {"###", "# #", "# #", "# #", "###"},
	'T': {"###", " # ", " # ", " # ", " # "},
	'E': {"###", "#  ", "## ", "#  ", "###"},
	'S': {" ##", "#  ", " # ", "  #", "## "},
	'C': {" ##", "#  ", "#  ", "#  ", " ##"},
	'N': {"# #", "###", "###", "# #", "# #"},
	'D': {"## ", "# #", "# #", "# #", "## "},
	'R': {"## ", "# #", "## ", "# #", "# #"},
	'A': {" # ", "# #", "###", "# #", "# #"},
	'M': {"# #", "###", "###", "# #", "# #"},
	'Y': {"# #", "# #", " # ", " # ", " # "},
	'P': {"## ", "# #", "## ", "#  ", "#  "},
	'L': {"#  ", "#  ", "#  ", "#  ", "###"},
	'Q': {" # ", "# #", "# #", "## ", " ##"},
	'U': {"# #", "# #", "# #", "# #", "###"},
	'G': {" ##", "#  ", "# #", "# #", " ##"},
	'B': {"## ", "# #", "## ", "# #", "## "},
	'K': {"# #", "# #", "## ", "# #", "# #"},
	'=': {"   ", "###", "   ", "###", "   "},
	'(': {"  #", " # ", " # ", " # ", "  #"},
	')': {"#  ", " # ", " # ", " # ", "#  "},
}

func drawText(img *image.RGBA, s string, x, y, scale int, c color.RGBA) {
	cx := x
	for _, r := range s {
		if r == ' ' {
			cx += 4 * scale
			continue
		}
		g, ok := pixFont[r]
		if !ok {
			// Majuscule inconnue -> on ignore (jamais bloquant)
			if r >= 'a' && r <= 'z' {
				g, ok = pixFont[r-32]
			}
		}
		if !ok {
			cx += 4 * scale
			continue
		}
		for row := 0; row < 5; row++ {
			for col := 0; col < 3; col++ {
				if g[row][col] != '#' {
					continue
				}
				for dy := 0; dy < scale; dy++ {
					for dx := 0; dx < scale; dx++ {
						img.SetRGBA(cx+col*scale+dx, y+row*scale+dy, c)
					}
				}
			}
		}
		cx += 4 * scale
	}
}
