// meltlock locks the screen, showing the current desktop melted into dots
// in the Catppuccin Mocha palette.
//
// With swaylock-plugin installed the dots are animated: meltlock hands it
// a coarse snapshot of each output and is started again by it, as
// `meltlock bg`, to draw the background. Without it, plain swaylock shows
// a still picture.
//
// The locker does the locking and the password check. meltlock only makes
// the picture, and always starts the locker even if the picture fails.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image/png"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

const usage = `usage: meltlock [command | LOCKER-OPTION...]

  (none)            lock the screen
  LOCKER-OPTION...  lock the screen, passing the options on to swaylock or
                    swaylock-plugin. With --ready-fd (-R) the locker stays
                    in the foreground, for a caller that supervises it
  preview [DIR]     write the still pictures without locking
  frames DIR [S..]  write every effect at the given seconds
  bg [EFFECT] [overlay]
                    draw the animated background (run by swaylock-plugin)

Effects: melt, sand, ripple, drift, life, pulse, silk, ridge, contour, frost.
One is picked at random for each lock. Switch effects off in ~/.config/meltlock/effects with lines
like "sand off"; MELTLOCK_EFFECT=melt,silk overrides the file.
`

// Indicator colours, rrggbb[aa]. Shared by swaylock and swaylock-plugin.
var lockArgs = []string{
	"--daemonize", "--ignore-empty-password", "--show-failed-attempts",
	"--scaling", "fill", "--color", "11111b",
	"--font", "FiraCode Nerd Font",
	"--indicator-radius", "80", "--indicator-thickness", "8",
	"--inside-color", "11111bdd", "--inside-clear-color", "11111bdd",
	"--inside-ver-color", "11111bdd", "--inside-wrong-color", "11111bdd",
	"--ring-color", "b4befe", "--ring-clear-color", "f9e2af",
	"--ring-ver-color", "89b4fa", "--ring-wrong-color", "f38ba8",
	"--key-hl-color", "cba6f7", "--bs-hl-color", "f38ba8",
	"--text-color", "cdd6f4", "--text-clear-color", "f9e2af",
	"--text-ver-color", "89b4fa", "--text-wrong-color", "f38ba8",
	"--line-uses-inside", "--separator-color", "00000000",
}

func main() {
	dir := filepath.Join(os.Getenv("XDG_RUNTIME_DIR"), "meltlock")
	arg := func(i int) string {
		if len(os.Args) > i {
			return os.Args[i]
		}
		return ""
	}
	var err error
	switch arg(1) {
	case "":
		err = lock(dir, nil)
	case "bg":
		layer, effect := uint32(layerBack), ""
		for _, a := range os.Args[2:] {
			if a == "overlay" {
				layer = layerOverlay
			} else {
				effect = a
			}
		}
		if effect == "" {
			effect = chooseEffect()
		}
		err = runBackground(dir, layer, effect)
	case "preview":
		if arg(2) != "" {
			dir = arg(2)
		}
		for _, image := range capture(dir, false) {
			fmt.Println(image)
		}
	case "frames":
		if arg(2) == "" {
			fmt.Fprint(os.Stderr, usage)
			os.Exit(2)
		}
		err = frames(arg(2), os.Args[3:])
	default:
		if !strings.HasPrefix(arg(1), "-") {
			fmt.Fprint(os.Stderr, usage)
			os.Exit(2)
		}
		err = lock(dir, os.Args[1:])
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "meltlock:", err)
		os.Exit(1)
	}
}

// lockerArgs are the locker's options: the shared ones, then the caller's
// extra ones, which win where they repeat an option. A caller that passes
// --ready-fd supervises the locker and learns from it when the screen is
// locked and, when it exits, unlocked, so it must not daemonize.
func lockerArgs(extra []string) []string {
	_, supervised := readyFD(extra)
	var args []string
	for _, a := range lockArgs {
		if a == "--daemonize" && supervised {
			continue
		}
		args = append(args, a)
	}
	return append(args, extra...)
}

// readyFD finds --ready-fd among the locker options, in the forms swaylock
// accepts. ok is set whenever the option is there; fd is -1 if its value is
// not a number.
func readyFD(extra []string) (fd int, ok bool) {
	for i, a := range extra {
		var v string
		switch {
		case a == "--ready-fd" || a == "-R":
			ok = true
			if i+1 < len(extra) {
				v = extra[i+1]
			}
		case strings.HasPrefix(a, "--ready-fd="):
			ok, v = true, strings.TrimPrefix(a, "--ready-fd=")
		case strings.HasPrefix(a, "-R"):
			ok, v = true, strings.TrimPrefix(a, "-R")
		default:
			continue
		}
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			return n, true
		}
		return -1, true
	}
	return -1, ok
}

func lock(dir string, extra []string) error {
	if lockers := runningLockers(); len(lockers) > 0 {
		// Already locked. A supervisor still needs to hear that the screen
		// is locked, and must not take a quick exit for an unlock, so stand
		// in for the running locker until it is gone.
		if fd, _ := readyFD(extra); fd >= 0 {
			ready := os.NewFile(uintptr(fd), "ready-fd")
			_, err := ready.WriteString("\n")
			ready.Close()
			if err != nil {
				return err
			}
			waitGone(lockers, time.Second)
		}
		return nil
	}
	self, _ := os.Executable()
	if plugin := findPlugin(self); plugin != "" {
		capture(dir, true)
		args := append([]string{"swaylock-plugin"}, lockerArgs(extra)...)
		args = append(args, "--command-each", self+" bg "+chooseEffect())
		return syscall.Exec(plugin, args, os.Environ())
	}
	args := append([]string{"swaylock"}, lockerArgs(extra)...)
	for _, image := range capture(dir, false) {
		args = append(args, "--image", image)
	}
	path, err := exec.LookPath("swaylock")
	if err != nil {
		return err
	}
	return syscall.Exec(path, args, os.Environ())
}

// locker is a running locker process, by PID and name, so that a reused
// PID is not mistaken for it.
type locker struct {
	pid  int
	name string
}

func runningLockers() []locker {
	var found []locker
	for _, name := range []string{"swaylock", "swaylock-plugin"} {
		out, _ := exec.Command("pgrep", "-x", name).Output()
		for _, f := range strings.Fields(string(out)) {
			if pid, err := strconv.Atoi(f); err == nil {
				found = append(found, locker{pid, name})
			}
		}
	}
	return found
}

// waitGone returns once none of the lockers runs. The lockers are not our
// children, so there is nothing to wait on but /proc.
func waitGone(lockers []locker, every time.Duration) {
	for _, l := range lockers {
		for {
			comm, err := os.ReadFile(fmt.Sprintf("/proc/%d/comm", l.pid))
			if err != nil || strings.TrimSpace(string(comm)) != l.name {
				break
			}
			time.Sleep(every)
		}
	}
}

// chooseEffect picks the effect for this lock: one of the comma-separated
// names in MELTLOCK_EFFECT, or else one that the effects file leaves on.
func chooseEffect() string {
	spec := os.Getenv("MELTLOCK_EFFECT")
	if spec == "" {
		dir := os.Getenv("XDG_CONFIG_HOME")
		if dir == "" {
			dir = filepath.Join(os.Getenv("HOME"), ".config")
		}
		conf, _ := os.ReadFile(filepath.Join(dir, "meltlock", "effects"))
		spec = enabledEffects(string(conf))
	}
	return pickEffect(spec, rand.New(rand.NewSource(time.Now().UnixNano())))
}

// enabledEffects reads the effects file: one "name on" or "name off" per
// line, # starts a comment. Effects it does not mention are on. The result
// is a list for pickEffect.
func enabledEffects(conf string) string {
	off := map[string]bool{}
	for _, line := range strings.Split(conf, "\n") {
		line, _, _ = strings.Cut(line, "#")
		if f := strings.Fields(strings.ToLower(strings.ReplaceAll(line, "=", " "))); len(f) >= 2 {
			off[f[0]] = f[1] == "off" || f[1] == "no" || f[1] == "false" || f[1] == "0"
		}
	}
	var on []string
	for _, e := range effects {
		if !off[e.name] {
			on = append(on, e.name)
		}
	}
	return strings.Join(on, ",")
}

// findPlugin looks for swaylock-plugin next to meltlock and on the PATH.
// MELTLOCK_STATIC=1 forces the still picture.
func findPlugin(self string) string {
	if os.Getenv("MELTLOCK_STATIC") != "" {
		return ""
	}
	beside := filepath.Join(filepath.Dir(self), "swaylock-plugin")
	if info, err := os.Stat(beside); self != "" && err == nil && info.Mode()&0o111 != 0 {
		return beside
	}
	path, _ := exec.LookPath("swaylock-plugin")
	return path
}

// capture screenshots every output into dir. With animated set it stores a
// snapshot per output for `meltlock bg`; otherwise it renders still
// pictures and returns them as output:path. Outputs that fail are left
// out and get the locker's plain colour.
func capture(dir string, animated bool) []string {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	now := time.Now()
	outs := outputs(ctx)
	images := make([]string, len(outs))
	var wg sync.WaitGroup
	for i, out := range outs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			snap, png := filepath.Join(dir, out+".snap"), filepath.Join(dir, out+".png")
			g, w, h, err := grab(ctx, out)
			switch {
			case err != nil:
				os.Remove(snap) // never animate a stale desktop
			case animated:
				err = g.snapshot().save(snap)
			default:
				g.snapshot().save(snap) // lets `meltlock bg` be tried by hand
				g.melt(rand.New(rand.NewSource(now.UnixNano() + int64(i))))
				g.stamp("LOCKED SINCE", now.Format("15:04"))
				if err = savePNG(png, g, w, h); err == nil {
					images[i] = out + ":" + png
				}
			}
			if err != nil {
				fmt.Fprintf(os.Stderr, "meltlock: %s: %v\n", out, err)
			}
		}()
	}
	wg.Wait()
	var ok []string
	for _, image := range images {
		if image != "" {
			ok = append(ok, image)
		}
	}
	return ok
}

func outputs(ctx context.Context) []string {
	raw, err := exec.CommandContext(ctx, "swaymsg", "-t", "get_outputs", "-r").Output()
	if err != nil {
		return nil
	}
	var outs []struct {
		Name   string `json:"name"`
		Active bool   `json:"active"`
	}
	json.Unmarshal(raw, &outs)
	var names []string
	for _, o := range outs {
		if o.Active {
			names = append(names, o.Name)
		}
	}
	return names
}

// grab screenshots one output and reduces it to the cell grid.
func grab(ctx context.Context, out string) (g *grid, w, h int, err error) {
	// MELTLOCK_SHOT names a PPM file to use in place of a screenshot, for
	// judging effects on a fixed picture.
	var shot []byte
	if file := os.Getenv("MELTLOCK_SHOT"); file != "" {
		shot, err = os.ReadFile(file)
	} else {
		shot, err = exec.CommandContext(ctx, "grim", "-o", out, "-t", "ppm", "-").Output()
	}
	if err != nil {
		return nil, 0, 0, err
	}
	pix, w, h, err := readPPM(bytes.NewReader(shot))
	if err != nil {
		return nil, 0, 0, err
	}
	return sample(pix, w, h), w, h, nil
}

func savePNG(path string, g *grid, w, h int) error {
	tmp := path + ".tmp"
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	enc := png.Encoder{CompressionLevel: png.BestSpeed}
	if err := enc.Encode(f, g.draw(w, h)); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// frames writes the animation of the first output at a few points in time.
func frames(dir string, times []string) error {
	at := []float64{1, 8, 45, 240, 1200}
	if len(times) > 0 {
		at = nil
		for _, t := range times {
			v, err := strconv.ParseFloat(t, 64)
			if err != nil {
				return err
			}
			at = append(at, v)
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	outs := outputs(ctx)
	if len(outs) == 0 {
		return fmt.Errorf("no outputs")
	}
	g, _, _, err := grab(ctx, outs[0])
	if err != nil {
		return err
	}
	for _, e := range effects {
		if spec := os.Getenv("MELTLOCK_EFFECT"); spec != "" && pickEffect(spec, rand.New(rand.NewSource(1))) != e.name {
			continue
		}
		if err := dumpFrames(dir, g.snapshot(), at, e.name); err != nil {
			return err
		}
	}
	return nil
}
