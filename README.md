# meltlock

A lock screen for sway. It takes a screenshot of each output, reduces it to
a grid of dots in the Catppuccin Mocha palette, lets the bright edges drip,
stamps `LOCKED SINCE hh:mm` on top and hands the picture to `swaylock`.

The dots are 12 px, so nothing on the desktop stays readable, and every
lock looks different.

`swaylock` does the locking and the password check. meltlock only makes the
picture, in about 0.2 s, and starts swaylock even if the picture fails.

## Animated

With `swaylock-plugin` installed (`scripts/build-swaylock-plugin.sh`) the
picture moves, and the date and a running clock are shown. One effect is
picked at random for each lock:

| Effect | What happens | Drawn as |
|---|---|---|
| `melt` | Drips run down the picture and wear it away; a thick liquid heaps up at the bottom and runs off the sides | dots |
| `sand` | The bright parts come loose grain by grain, fall, and pile up in heaps | blocks |
| `ripple` | Drops send out rings that bend the picture while it sinks out of sight | diamonds |
| `drift` | A wind sorts each row by brightness until only gradients are left | bars |
| `life` | The bright parts come alive as Conway's Game of Life | blocks |
| `pulse` | The picture breathes in a slow swell from the centre while it softens into patches of colour | dots |
| `silk` | Fine threads flow along the outlines of the desktop and a slowly turning wind, in the colours beneath them | full resolution |
| `ridge` | The desktop as a landscape: lines rise where it is bright, hide the ones behind them and travel slowly upwards | full resolution |
| `contour` | The desktop as a map: contour lines follow its brightness and spread outwards in slow rings | full resolution |
| `frost` | Ice crystals grow over the picture, branching at sixty degrees, while it fades beneath them | full resolution |

Choose which effects take part in `~/.config/meltlock/effects`, one line
each:

    sand  off
    silk  on

Effects the file does not mention are on. `MELTLOCK_EFFECT=melt,sand`
overrides the file (one name pins it).
`MELTLOCK_STATIC=1` forces the still picture with plain swaylock.
`meltlock-trial` locks with an automatic unlock after 60 s.

## Use

| How | |
|---|---|
| `Mod+Escape` | bound in `~/.config/sway/config` |
| `Mod+d`, "Lock Screen" | desktop entry |
| `meltlock` | from a terminal |
| `meltlock --ready-fd FD [OPTION..]` | from a program that supervises the locker, such as [screensaver-bridge](https://github.com/oscarh/screensaver-bridge); options go on to swaylock, which then stays in the foreground |
| `meltlock preview [DIR]` | render the pictures without locking |
| `meltlock frames DIR [SECONDS..]` | render every effect at those times, without locking |
| `timeout 5 meltlock bg sand overlay` | watch an effect live for 5 s |

## Install

    make install

Needs `swaylock`, `grim` and `swaymsg`.

## Notes

- Pictures are written to `$XDG_RUNTIME_DIR/meltlock`, readable only by you.
- The time on the picture is when the screen was locked; plain swaylock
  cannot show a running clock.
- Indicator colours are the `lockArgs` list in `main.go`.
