# WhiskeesFX VisualFX: when each effect happens in real life

Research behind the triggers and strengths in visualfx.ini (v12, 19 Nov 2026).

| Effect (ini names) | When it shows up | Strongest | Weakest / gone | How v12 does it |
|---|---|---|---|---|
| Reverse spray, wet runway (`reverser_*`, `wing_water*`, `reverser_up_mist`) | Reverse selected on a wet runway | Max reverse right after touchdown | Airbus: back to idle reverse by 70 kt, stowed at taxi speed | Thickness follows the Fenix's real N1; layers switch on at N1 45 / 60 / 65 as before |
| Engine blast spray (`engine_spray`) | High power on a wet runway | Start of the take-off roll | Left behind as speed builds | Trail effect (stays where made), thickness and amount follow N1 |
| Wheel spray (`rain_wheel_spray*`, `wheel_spray*`, `rain_wheel_plume`) | Tyres rolling through water | Grows with speed and water depth; side plumes angled out and up | Little at taxi speed | Grows 10 to 140 kt; new droplet side plumes that fall back under gravity |
| Wet touchdown (`touchdown_splash*`) | Mains touching a wet runway | First half second | Over in about a second | One burst of droplets plus a mist puff, fresh every landing |
| Intake ground vortex (`intake_swirl`, `snow_intake_vortex`) | High power, very low speed, humid air (or snow/water lifted from the ground) | As power is set at the start of the take-off roll | A little rolling speed blows it away | Now fades out between 0 and 60 kt instead of switching off; snow version is a corkscrew column |
| Intake condensation (`intake_condensation`, `intake_fog`) | Humid air, high power | Low speed, high power | Disappears as the aircraft speeds up | Unchanged (approved) |
| Wingtip and flap vortices (`*_vortex*`) | Humid days, take-off and landing | High lift: flaps out, slow, high angle of attack | Cruise speeds, dry air | Unchanged look (approved), brighter as asked |
| Nacelle strake vortices (`strake_vortex*`) | Humid air, high angle of attack | Rotation, streaming up and over the wing leading edge | Low angle of attack | Unchanged (approved) |
| Over-wing vapour (`wing_vapour_*`) | Humid air, angle of attack (not g force by itself) | Rotation and flare | Low angle of attack | Unchanged (approved) |
| Pylon vapour (`pylon_vapour`, new) | Humid air on take-off | Rotation / initial climb | | New; position needs a marker check |
| Exhaust steam (`exhaust_steam`, `apu_steam`) | Cold air, water vapour from burning fuel condenses like breath | The colder the day | Warm days | Approved: below 5 C, very faint. APU version off until positioned |
| Heat haze (`heat_haze`) | Always behind a running engine | Higher power: stronger and longer | Idle | Custom heat-deform mesh (experimental), strength and size follow N1; Asobo version kept as fallback (off) |
| Start-up puff (`engine_start_puff`, new) | Light-off during engine start | Around light-off (N2 ~16-22 %) | Gone as the engine spools up (~30 % N2) | Short grey-white puff from the exhaust |
| Tyre smoke (`tyre_smoke`) | Dry touchdown: wheels skid while spinning up | Faster, firmer touchdowns | Spin-up takes about a second or less | Approved look; now restarts fresh each landing |
| Dry reverse dust (`reverse_dust_dry`, new) | Reverse on a dry runway | Max reverse | Much less than spray on wet | Very faint grey-brown haze |
| Water off the wing / wheels (`wing_rain_sheet`, `gear_fling`, `*_drips`) | Rain on the take-off roll; wheels spinning after a wet lift-off | Higher speed; just after lift-off | Gone by ~150 ft (fling), 600 ft (drips) | New sheet and fling; drips unchanged |
| Snow (`snow_*`) | Snow-covered runway | Max reverse: the cloud can hide the runway edges | | Same layers as rain, white, bigger and longer |
| De-icing fluid shedding (`deice_fluid_shed`, new) | Type II/IV fluid shears off during the take-off roll | From about 50 kt | Mostly gone by rotation (100-110 kt) | Faint mist off the trailing edge between 50 and 130 kt when freezing with precipitation |

## Vortices in detail (19 Nov, v12b)

- Shape: after roll-up the wake is two counter-rotating cylindrical vortices, most of the energy within a few feet of the core. Seen in humid air the condensed core looks like a long helical (corkscrew) trail, not a straight line or rings.
- Sinking: at altitude they sink several hundred ft/min (FAA: 300-500 fpm for about 30 s), slowing over time. For an A321 on approach (about 75 t, 70 m/s, 35.8 m span) the classic estimate gives roughly 1.8 m/s.
- Wind: they move with the air. Away from the ground they stay a bit less than a span apart and drift with the wind; a 1-5 kt crosswind can hold the upwind one over the runway.
- Near the ground (within 100-200 ft, about one span): sinking stops and they move apart over the ground at 2-3 kt each, decaying faster.
- Roll-up: each tip vortex settles about pi/4 of the span apart (slightly inboard of the tips). Flap-edge and tip vortices turn the same way and wind round each other into one.
- Longer term (tens of seconds, beyond what condensation shows): Crow instability makes the pair wave and link into rings.
- v12b settings: corkscrew from a 25 rad/s sin/cos of time, slow meander, 1.2 m/s inboard roll-up (tip) and 1.0 m/s outboard (flap) so they converge, about 2.2 m/s sinking that turns into a 1.3 m/s outward spread below about 120 ft AGL, gm_drag 0.4 so the motions ease off, gm_wind 1 so the threads drift with the sim's wind.

Vortex sources: FAA wake turbulence briefing https://www.faa.gov/sites/faa.gov/files/training_testing/training/wake/08APP3C.PDF ; FAA AC 90-23G https://www.faa.gov/documentLibrary/media/Advisory_Circular/AC_90-23G.pdf ; AIM 7-4 https://www.faa.gov/air_traffic/publications/atpubs/aim_html/chap7_section_4.html ; NASA in-ground-effect decay https://ntrs.nasa.gov/api/citations/20140011455/downloads/20140011455.pdf ; NASA vortex models (b0 = B pi/4, V0 = Gamma0/2 pi b0) https://ntrs.nasa.gov/api/citations/20140003974/downloads/20140003974.pdf ; Leweke, DTU (tip + flap vortex merging) https://fluid.dtu.dk/-/media/a92a0af374f84f8baf0b69f62c6aa651.pdf ; Atmospheric Optics (helical trails) https://old.atoptics.co.uk/fz1135.htm ; Crow instability https://en.wikipedia.org/wiki/Crow_instability

## Sources

- Airbus FCTM via PPRuNe (A320): reverse thrust less efficient below 70 kt, REV IDLE at 70 kt, stow at taxi speed. https://www.pprune.org/archive/index.php/t-623089.html
- NLR / ICAS: tyre spray patterns (bow wave, side plumes; speed, tyre pressure and water depth effects). https://icas.org/ICAS_ARCHIVE/ICAS2012/PAPERS/268.PDF
- Aerospace Science & Technology 2024: spray (bow wave + side plume) increases with tyre speed and water depth. https://searchworks.jioinstitute.edu.in/articles/asx__176499868
- NASA TM X-72695: nose gear spray characteristics vs speed and water depth. https://ntrs.nasa.gov/api/citations/19750015483/downloads/19750015483.pdf
- WPI thesis: inlet vortices often observed during engine power-up at the start of the take-off run. https://digital.wpi.edu/downloads/9p290945b
- ICAS 2020: a small forward rolling speed can eliminate the ground-effect vortex. https://www.icas.org/icas_archive/ICAS2020/data/papers/ICAS2020_0504_paper.pdf
- NATO STO AVT-195: ground vortex at high power, disappears as the aircraft takes up speed. https://www.sto.nato.int/publications/STO%20Educational%20Notes/RTO-EN-AVT-195/EN-AVT-195-01.pdf
- Wikipedia, Wingtip vortices: condensation most common at high angle of attack, airliners taking off and landing on humid days. https://en.wikipedia.org/wiki/Wingtip_vortices
- Ask the Pilot / AirlineRatings: vortices strongest on approach and departure; vapour off flap fairings and pylons; over-wing puffs. https://askthepilot.com/?p=3041
- Rob Edgcumbe: nacelle strake vortex strengthens at rotation and streams over the wing leading edge. https://robedgcumbe.com/?p=21816
- ERAU / Springer: tyre smoke from skidding while the wheels spin up; main tyre spin-up about 1.3 s (BAe 146). https://link.springer.com/content/pdf/10.1007/s13272-024-00729-7.pdf
- CFM56-3 training manual: fuel collected in the exhaust can ignite at start. https://www.scribd.com/document/354006637/TRAINING-MANUAL-CFM56-3-pdf
- NASA icing guide: Type II/IV fluids stay on during taxi and flow off under wind shear on the take-off roll; Type IV for rotation speeds of 100-110 kt or more. https://aircrafticing.grc.nasa.gov/1_2_3_1.html
- TSB / AeroInside: reverse thrust blowing up snow and causing a whiteout on landing. https://www.aeroinside.com/3579/provincial-sf34-at-stephenville-on-jan-8th-2014-went-into-snow-drift-on-landing
- Contrail / exhaust condensation basics (water vapour in exhaust condensing in cold air). https://en.wikipedia.org/wiki/Contrail

## Wake vortices and wing vapour: reference video (20 Nov)
"Emirates 777 wake vortex spectacular!" (flugsnug, youtube.com/watch?v=ViKYFsN3p24), picked by Whiskee as the target look.
- Vortices: right behind the wingtip/flap edge the condensation is a clean, narrow, smooth TUBE (the low-pressure core),
  not smoke. Further back it twists, thickens and breaks up into chaotic, wispy tufts.
  -> vortex_near = the tube (dense, narrow, steady, ~1 s); vortex = the break-up (fast early growth, wander, corkscrew, wind).
- Wing vapour: thicker and faster than ours, and it flickers at random, each wing on its own.
  -> vapour faster (-65 m/s) and bigger; alpha x1.35 with a per-side flicker (0.1 to 1.1).
Old values: VisualFX/visualfx_before_tube.ini.
