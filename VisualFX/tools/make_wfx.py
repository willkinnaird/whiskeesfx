"""WhiskeesFX VisualFX v2 effects: every effect is our own, with live dials.

  python tools/make_wfx.py [EFFECT_LIB_DIR]

Writes WFX_Mist, WFX_Trail, WFX_Droplets and WFX_HeatHaze (+ _edition files) and the
VisualEffectLibrary.vfxlib into the Dev Mode project, keeping the older effects
(WingWater, GenericMist, GenericTrail, GenericRibbon, Marker) so old settings still work.

WFX_Mist      particles move WITH the aircraft (local space): spray, steam, vapour, condensation
WFX_Trail     particles stay where they were born (world space): wheel spray, vortices, smoke, drips
WFX_Droplets  WFX_Mist with the water-splash texture: heavier droplets for splashes and spray cores
WFX_HeatHaze  heat-shimmer mesh (SDK heat-deform example), experimental

Every particle effect reads the same dials (graph parameters, all live from visualfx.ini):

  where it starts   gm_spread_x/y/z      half-size of the start box (m)
  how it moves      gm_dir_x/y/z         base velocity (m/s), x sideways (+ = aircraft left), y up, z forward
                    gm_jitter            random +- added to each axis (m/s)
                    gm_speed             extra speed (m/s) along a cone:
                    gm_pitch, gm_heading cone axis (degrees)
                    gm_cone, gm_cone_in  cone outer / inner half-angle (degrees)
                    gm_gravity           m/s2 pulling down (negative = rises like warm air)
                    gm_drag              how fast particles slow down (1/s, 0 = never)
                    gm_wind              0..1 how much they drift with the wind
                    gm_swirl             corkscrew speed (m/s); gm_swirl_rate turns per second (rad/s);
                    gm_swirl_up          0 = corkscrew around the fore-aft axis, 1 = around the vertical
  how big           gm_size (m), gm_size_min (0..1 smallest random share), gm_grow (x1+grow over life),
                    gm_grow_curve (1 = steady, 0.5 = fast then slow like smoke, 2 = slow then fast)
  how long          gm_life (s), gm_life_min (0..1 shortest random share)
  how thick         gm_alpha (peak opacity), gm_shape (0 = old soft peak mid-life, 1 = fade in / hold / fade out),
                    gm_fade_in, gm_fade_out (share of the life, with gm_shape 1)
  colour            gm_grey (1 white .. 0 black), gm_r/g/b (0..1 each), gm_blue (0..1 blue tint), gm_glow (self-lit)
  spin              gm_spin (random spin either way, rotation units per second)
  how many          gm_rate (particles/s), gm_burst (extra /s right after it starts), gm_burst_time (s)
"""
import os, sys, uuid

OUT = sys.argv[1] if len(sys.argv) > 1 else os.path.join(
    os.path.dirname(os.path.dirname(os.path.abspath(__file__))), "effects", "PackageSources", "VisualEffectLibs",
    "whiskeesfx-visualfx", "VisualFX")
Z = "{00000000-0000-0000-0000-000000000000}"
MIST = "{ABF127FE-7F13-45AF-A31B-9A6FFD02C00F}"     # VFX_Water_Mist (the material all our approved effects use)
SPLASH = "{E344CBC7-6293-478C-983C-F5DA49C6D62C}"   # VFX_Water_Splash
# Our own material (MaterialLibs/whiskeesfx-materials/WFX_Ring.material): a soft ring with a clear middle, so each
# camera-facing particle is a ring and a dense line of them reads as stacked circles / a hollow tube.
RINGMAT = "{9D3B6E52-4A71-4C08-8F2D-1E6B5A7C3D94}"
# 23 Nov: WFX_Tube material - bright soft edges, 30% middle across the strip; drawn as a ribbon it reads as a tube
TUBEMAT = "{4C8E1A27-93B5-4D60-A7F2-6E0B9D3C5A18}"
# From the SDK's StaticMesh / Heat Deform XML example.
HEAT_MATERIAL = "{DE37F56D-442C-45AB-A325-4572668CE135}"
HEAT_MESH = "{58F995B0-49DE-40FD-B839-881844D30EAD}"

NS = uuid.UUID("3c1e9d52-7a40-4b8e-a3f1-5d2c7b0e9a64")
EFFECTS = {   # name: (effect guid, kind, material, local space, capacity)
    "WFX_Mist":     ("{4D2A7C10-8E35-4B61-9F02-6A1C3E5B7D81}", "particles", MIST, True, 2500),
    "WFX_Trail":    ("{7B3E1F42-2C9D-4A70-8E51-0D4F6A2B9C13}", "particles", MIST, False, 5000),
    "WFX_Droplets": ("{A61C5E93-4F27-4D08-B3E6-91B2D7C40F55}", "particles", SPLASH, True, 1500),
    "WFX_HeatHaze": ("{E2B94D17-6A3C-4F85-9D10-3C7E5A8B2F69}", "heat", None, True, 0),
    # 21 Nov: hollow tubes (vortex ropes, intake tornados). WFX_Trail, but each particle starts on a circle of
    # radius gm_ring around the fore-aft axis and moves outward at gm_ring_speed, so a dense stream of small dots
    # reads as stacked rings / a hollow tube from any angle. No custom texture needed.
    "WFX_Ring":     ("{C3A85E21-7D49-4F06-B2E8-5A1D9C6F3B47}", "ring", MIST, False, 20000),
    # 21 Nov: WFX_Trail with our ring-texture material (world space).
    "WFX_RingTex":  ("{F1C7A3D8-2B64-4E95-9A30-6D8E2F1B5C07}", "particles", RINGMAT, False, 20000),
    # 23 Nov: the vortex tube - WFX_Trail drawn as a RIBBON (particles joined into one continuous camera-facing strip,
    # gm_size = width) with the tube texture, so it's one solid see-through tube from any angle.
    "WFX_Tube":     ("{9B2E7D40-5C13-4A86-8F91-3D6A0E2B7C55}", "ribbon", TUBEMAT, False, 20000),
    # "Safe" twins: the same dials, built only from node types already proven in released MSFS 2024 effects.
    # No gravity-vector node (gravity pulls along the effect's own -y), no birth-velocity read-back,
    # no age node (age = life share x life), no spin, no glow. If a full effect fails to spawn, point the
    # preset guids in visualfx.ini at these.
    "WFX_MistSafe":     ("{5E8B2D31-9C46-4A17-8F63-2B7D0E4C1A92}", "safe", MIST, True, 2500),
    "WFX_TrailSafe":    ("{8C4F2A53-3D1E-4B81-9A62-1E5B7C3D0F24}", "safe", MIST, False, 5000),
    "WFX_DropletsSafe": ("{B72D6F04-5A38-4E19-C4F7-02C3E8D51A66}", "safe", SPLASH, True, 1500),
}

DIALS = [  # name, default (defaults reproduce the old GenericMist look)
    ("gm_dir_x", 0.0), ("gm_dir_y", 1.0), ("gm_dir_z", 0.0), ("gm_jitter", 0.5),
    ("gm_spread_x", 0.5), ("gm_spread_y", 0.2), ("gm_spread_z", 0.5),
    ("gm_speed", 0.0), ("gm_pitch", 0.0), ("gm_heading", 0.0), ("gm_cone", 15.0), ("gm_cone_in", 0.0),
    ("gm_gravity", 0.0), ("gm_drag", 0.0), ("gm_wind", 0.0),
    ("gm_swirl", 0.0), ("gm_swirl_rate", 6.0), ("gm_swirl_up", 0.0),
    ("gm_size", 1.0), ("gm_size_min", 0.6), ("gm_grow", 0.0), ("gm_grow_curve", 1.0),
    ("gm_life", 1.0), ("gm_life_min", 0.7),
    ("gm_alpha", 0.2), ("gm_shape", 0.0), ("gm_fade_in", 0.15), ("gm_fade_out", 0.4),
    ("gm_grey", 1.0), ("gm_r", 1.0), ("gm_g", 1.0), ("gm_b", 1.0), ("gm_blue", 0.0), ("gm_glow", 0.0),
    ("gm_spin", 0.0),
    ("gm_rate", 160.0), ("gm_burst", 0.0), ("gm_burst_time", 0.5),
]
RING_DIALS = [("gm_ring", 0.4), ("gm_ring_speed", 0.0), ("gm_ring_up", 0.0)]   # radius (m), outward speed (m/s), 1 = ring lies flat (vertical tube)

PREFIX = ""
nodes, positions = [], {}


def gid(name):
    return "{" + str(uuid.uuid5(NS, PREFIX + name)).upper() + "}"


def f(x):
    return f"{x:.6f}"


def pin(tag, link=None, value=0.0):
    return f"            <{tag}>{link or Z}, {f(value)}</{tag}>\n"


def lv(x):
    """(link, value) for a node input: a node output id or a constant."""
    return (x, 0.0) if isinstance(x, str) else (None, float(x))


_y = [0]


def node(kind, name, body, pos=None):
    iid = gid(name)
    out = gid(name + ".out")
    nodes.append(f'        <VisualEffect.{kind} InstanceId="{iid}">\n'
                 f"            <OutputValue>{out}</OutputValue>\n{body}        </VisualEffect.{kind}>\n")
    if pos is None:
        _y[0] += 45
        pos = (-600 + 220 * (len(nodes) % 6), _y[0])
    positions[iid] = pos
    return out


def op(kind, name, a, b):
    (la, va), (lb, vb) = lv(a), lv(b)
    return node(kind + "Operation", name, pin("xVariant", la, va) + pin("yVariant", lb, vb))


def mul(n, a, b): return op("Multiply", n, a, b)
def add(n, a, b): return op("Add", n, a, b)
def sub(n, a, b): return op("Subtract", n, a, b)
def div(n, a, b): return op("Divide", n, a, b)
def pw(n, a, b): return op("Pow", n, a, b)


def clamp(n, x, lo, hi):
    l, v = lv(x)
    return node("Clamp", n, pin("x", l, v) + pin("y", None, lo) + pin("z", None, hi))


def lerp(n, a, b, t, smooth=False):
    (la, va), (lb, vb), (lt, vt) = lv(a), lv(b), lv(t)
    return node("Lerp", n, ("            <LerpType>SmoothStep</LerpType>\n" if smooth else "")
                + pin("xVariant", la, va) + pin("yVariant", lb, vb) + pin("FXTime", lt, vt))


def vec(n, comps):
    kind = {2: "Vector2", 3: "Vector3", 4: "Vector4"}[len(comps)]
    body = ""
    for c, x in zip("xyzw", comps):
        l, v = lv(x)
        body += pin(c, l, v)
    return node(kind, n, body)


def trig(kind, n, x):
    l, v = lv(x)
    return node(kind, n, pin("x", l, v))


def attr(n, kind=None, source=False):
    body = ""
    if source:
        body += "            <ParticleLocationType>Source</ParticleLocationType>\n"
    if kind:   # Age is the default; the editor leaves the line out
        body += f"            <ParticleAttributeType>{kind}</ParticleAttributeType>\n"
    return node("GetParticleAttribute", n, body)


def param(name, default):
    return node("GraphParameter", name, f"            <GraphParamName>{name}</GraphParamName>\n"
                + pin("GraphParamDefaultValue", None, default))


_seed = [0]


def rand(n, lo, hi, pid):
    _seed[0] += 1
    (ll, vl), (lh, vh) = lv(lo), lv(hi)
    return node("RandomValue", n, pin("MinRandValue", ll, vl) + pin("MaxRandValue", lh, vh)
                + pin("RandSeed", None, _seed[0] * 7919.0) + pin("RandIndex", pid, _seed[0]))


def float_in(link, value=0.0):
    return f"                <FloatIn>{link or Z}, {f(value)}</FloatIn>\n"


def float3_in(link):
    return f"                <Float3In>{link or Z}, 0.000000, 0.000000, 0.000000</Float3In>\n"


def build_particles(name, guid, material, local, capacity, full=True, ring=False, oriented=False, ribbon=False):
    rel = oriented or ribbon
    P = {n: param(n, d) for n, d in DIALS + (RING_DIALS if ring else [])}
    pid = attr("pid", "id")
    t = attr("t", "AgeOverLifetime")

    # ---- start position: random in a box (+ on a circle for the ring effect)
    px = mul("px", rand("rpx", -1, 1, pid), P["gm_spread_x"])
    py = mul("py", rand("rpy", -1, 1, pid), P["gm_spread_y"])
    pz = mul("pz", rand("rpz", -1, 1, pid), P["gm_spread_z"])
    if ring:
        th = rand("rth", 0.0, 6.283185, pid)
        rc, rs = trig("Cos", "rcos", th), trig("Sin", "rsin", th)
        flat = P["gm_ring_up"]                       # 0: circle in the x-y plane (around fore-aft), 1: x-z plane
        ry = mul("ry", rs, sub("rnotup", 1.0, flat))
        rz = mul("rz", rs, flat)
        px = add("rpx2", px, mul("rx", rc, P["gm_ring"]))
        py = add("rpy2", py, mul("ryr", ry, P["gm_ring"]))
        pz = add("rpz2", pz, mul("rzr", rz, P["gm_ring"]))
    pos = vec("pos", [px, py, pz])

    # ---- start velocity: base direction + jitter + speed along a random direction in a cone
    def vj(axis):
        return add("v" + axis, P["gm_dir_" + axis], mul("j" + axis, rand("rj" + axis, -1, 1, pid), P["gm_jitter"]))
    if rel:
        # 24 Nov: aircraft-relative push for the world-space vortex effects. A world-space particle's velocity is in
        # world axes, so "dir_z -40" meant a fixed world direction, not aft. Build it from the emitter's own axes
        # (the same EmitterDirection node that orients the rings): x = sideways, y = aircraft up, z = fore-aft.
        ex = node("EmitterDirection", "eaxx", "            <EmitterDirectionType>X+</EmitterDirectionType>\n")
        ey = node("EmitterDirection", "eaxy", "            <EmitterDirectionType>Y+</EmitterDirectionType>\n")
        ez = node("EmitterDirection", "eaxz", "            <EmitterDirectionType>Z+</EmitterDirectionType>\n")
        base = add("vbase", add("vbxy", mul("vbx", ex, vj("x")), mul("vby", ey, vj("y"))), mul("vbz", ez, vj("z")))
    else:
        base = vec("vbase", [vj("x"), vj("y"), vj("z")])
    cone = node("RandomDirectionInCone", "cone",
                pin("InputPitch", P["gm_pitch"]) + pin("InputHeading", P["gm_heading"])
                + pin("InnerAngle", P["gm_cone_in"]) + pin("OuterAngle", P["gm_cone"])
                + pin("RandSeed", pid) + pin("RandIndex", None, 77))
    vel0 = add("vel0", base, mul("vcone", cone, P["gm_speed"]))
    if ring:   # outward from the centre line, so the ring widens with age
        vel0 = add("vel0r", vel0, vec("vring", [mul("vrx", rc, P["gm_ring_speed"]), mul("vry", ry, P["gm_ring_speed"]),
                                                  mul("vrz", rz, P["gm_ring_speed"])]))

    # ---- life and size at birth
    life = mul("life", rand("rlife", P["gm_life_min"], 1.0, pid), P["gm_life"])
    size0 = mul("size0", rand("rsize", P["gm_size_min"], 1.0, pid), P["gm_size"])
    rot0 = vec("rot0", [0.0, 0.0, rand("rrot", 0.0, 360.0, pid)]) if full else None
    if full:
        age = attr("age")                                  # seconds since this particle was born
        src_size = attr("srcsize", "Size", source=True)    # its size at birth
        src_vel = attr("srcvel", "Velocity", source=True)  # its velocity at birth (in the emitter's space)
    else:
        age = mul("age", t, life)                          # the same randoms give the same life every frame
        src_size, src_vel = size0, vel0

    # ---- over life: velocity = birth velocity slowed by drag + gravity + wind + corkscrew
    slow = div("slow", 1.0, add("drag1", mul("dragt", P["gm_drag"], age), 1.0))
    v_drag = mul("vdrag", src_vel, slow)
    if full:
        grav = node("GravityVector", "grav", pin("GravityScale", P["gm_gravity"], 0.0))
    else:
        grav = vec("grav", [0.0, mul("gneg", P["gm_gravity"], -1.0), 0.0])
    v_grav = mul("vgrav", grav, age)
    wind = node("WindDirection", "wind", "")
    v_wind = mul("vwind", wind, P["gm_wind"])
    phase = add("phase", mul("phw", P["gm_swirl_rate"], age), rand("rphase", 0.0, 6.283, pid))
    sa = mul("swa", trig("Cos", "cosp", phase), P["gm_swirl"])
    sb = mul("swb", trig("Sin", "sinp", phase), P["gm_swirl"])
    v_swirl = vec("vswirl", [sa, mul("swy", sb, sub("notup", 1.0, P["gm_swirl_up"])), mul("swz", sb, P["gm_swirl_up"])])
    vel = add("vel", add("velA", v_drag, v_grav), add("velB", v_wind, v_swirl))

    # ---- size over life: x (1 + grow * t^curve)
    grow = add("grow1", mul("growt", pw("tcurve", t, P["gm_grow_curve"]), P["gm_grow"]), 1.0)
    usize = mul("usize", src_size, grow)

    # ---- spin
    if full:
        spin_dir = rand("rspin", -1.0, 1.0, pid)
        rot = vec("rot", [0.0, 0.0, add("rotz", rand("rrot2", 0.0, 360.0, pid),
                                         mul("spinA", mul("spinB", P["gm_spin"], spin_dir), age))])

    # ---- opacity: old soft peak (4 t (1-t)) or fade in / hold / fade out, by gm_shape
    bump = mul("bump4", sub("bump", t, mul("tt", t, t)), 4.0)
    fin = lerp("fins", 0.0, 1.0, clamp("finc", div("fint", t, add("fi0", P["gm_fade_in"], 0.001)), 0.0, 1.0), smooth=True)
    fout = lerp("fouts", 0.0, 1.0, clamp("foutc", div("foutt", sub("left", 1.0, t), add("fo0", P["gm_fade_out"], 0.001)), 0.0, 1.0), smooth=True)
    trap = mul("trap", fin, fout)
    alpha = mul("alpha", lerp("shape", bump, trap, P["gm_shape"]), P["gm_alpha"])

    # ---- colour: grey x (r, g, b), blue tint pulls red and green down
    notblue = sub("notblue", 1.0, P["gm_blue"])
    cr = mul("cr", mul("cr0", P["gm_grey"], P["gm_r"]), notblue)
    cg = mul("cg", mul("cg0", P["gm_grey"], P["gm_g"]), notblue)
    cb = mul("cb", P["gm_grey"], P["gm_b"])
    colour = vec("colour", [cr, cg, cb, alpha])
    emissive = None if not full else vec("emissive", [mul("er", cr, P["gm_glow"]), mul("eg", cg, P["gm_glow"]), mul("eb", cb, P["gm_glow"]), 1.0])

    # ---- how many: steady rate + a burst that fades out over gm_burst_time after the effect starts
    inst_age = node("GetInstanceAttribute", "instage", "            <InstanceAttributeType>Age</InstanceAttributeType>\n")
    burst_left = clamp("burstleft", sub("burst1", 1.0, div("burstt", inst_age, add("bt0", P["gm_burst_time"], 0.001))), 0.0, 1.0)
    rate = add("rate", P["gm_rate"], mul("burst", burst_left, P["gm_burst"]))

    # ---- fixed orientation (rings): each quad stays square-on to the aircraft's fore-aft axis instead of turning to
    # face the camera, so a line of rings reads as a tube from the side (MSFS 2024 QuadBillboardType NoBillboarding;
    # the quad is perpendicular to ParticleQuadBillboardAxis)
    init_orient = out_orient = ""
    if oriented:
        q_axis = node("EmitterDirection", "qaxis", "            <EmitterDirectionType>Z+</EmitterDirectionType>\n")
        # (ParticleQuadBillboardUp isn't an accepted Init element - the build failed with it; a ring is round, so the
        # in-plane "up" doesn't matter)
        # format as the sim's VFX editor writes it (Float3In is rejected here: "Schema does not allow this operation")
        init_orient = ("            <ParticleQuadBillboardAxis>\n"
                       f"                <QuadBillboardAxis>{q_axis}, 0.000000, 0.000000, 1.000000</QuadBillboardAxis>\n"
                       "            </ParticleQuadBillboardAxis>\n")
        out_orient = "            <QuadBillboardType>NoBillboarding</QuadBillboardType>\n"
    if ribbon:
        out_orient += "            <PrimitiveType>Ribbon</PrimitiveType>\n"

    emitter, init, update, output = gid("emitter"), gid("init"), gid("update"), gid("output")
    init_rot = f"            <ParticleRotation>\n{float3_in(rot0)}            </ParticleRotation>\n" if full else ""
    upd_rot = f"            <ParticleRotation>\n{float3_in(rot)}            </ParticleRotation>\n" if full else ""
    emis_block = (f"            <MaterialEmissiveColor>\n                <ColorIn>{emissive}, 0, 0, 0, 0</ColorIn>\n"
                  "            </MaterialEmissiveColor>\n") if full else ""
    blocks = f'''        <VisualEffect.VisualEffect InstanceId="{guid}">
            <Name>{name}</Name>
            <EmitterList>
                <ObjectReference InstanceId="{emitter}" id="Emitter"/>
            </EmitterList>
        </VisualEffect.VisualEffect>
        <VisualEffect.Emitter InstanceId="{emitter}">
            <Capacity>{capacity}</Capacity>
            <ParticleRate>{rate}, 160.000000</ParticleRate>
            <EmitInLocalSpace>{"True" if local else "False"}</EmitInLocalSpace>
            <ParticleInit>
                <ObjectReference InstanceId="{init}" id="BlockParticleInit"/>
            </ParticleInit>
        </VisualEffect.Emitter>
        <VisualEffect.BlockParticleInit InstanceId="{init}">
            <ParticleUpdate>
                <ObjectReference InstanceId="{update}" id="BlockParticleUpdate"/>
            </ParticleUpdate>
            <ParticleLifetime>
{float_in(life, 1.0)}            </ParticleLifetime>
            <ParticleSize>
{float_in(size0, 0.4)}            </ParticleSize>
            <ParticlePosition>
{float3_in(pos)}            </ParticlePosition>
{init_rot}            <ParticleVelocity>
{float3_in(vel0)}            </ParticleVelocity>
{init_orient}        </VisualEffect.BlockParticleInit>
        <VisualEffect.BlockParticleUpdate InstanceId="{update}">
            <ParticleOutput>
                <ObjectReference InstanceId="{output}" id="Output"/>
            </ParticleOutput>
            <ParticleSize>
{float_in(usize, 0.4)}            </ParticleSize>
{upd_rot}            <ParticleVelocity>
{float3_in(vel)}            </ParticleVelocity>
        </VisualEffect.BlockParticleUpdate>
        <VisualEffect.Output InstanceId="{output}">
            <Material>{material}</Material>
{out_orient}            <ParticleColor>
                <ColorIn>{colour}, 0, 0, 0, 0</ColorIn>
            </ParticleColor>
{emis_block}        </VisualEffect.Output>
'''
    for k, (iid, p) in enumerate([(guid, (300, -100)), (emitter, (300, 0)), (init, (300, 200)), (update, (300, 420)), (output, (300, 620))]):
        positions[iid] = p
    return blocks


def build_heat(name, guid):
    """Heat shimmer: the SDK's heat-deform static mesh, sized and aimed from the ini."""
    hh_strength, hh_speed = param("hh_strength", 2.5), param("hh_speed", 1.75)
    hh_blur, hh_scale = param("hh_blur", 1.0), param("hh_scale", 1.0)
    hh_p, hh_b, hh_h = param("hh_pitch", -90.0), param("hh_bank", 55.0), param("hh_heading", 90.0)
    hh_x, hh_y, hh_z = param("hh_x", 0.0), param("hh_y", 0.0), param("hh_z", 0.0)
    deform = vec("deform", [hh_strength, hh_speed])
    rot = vec("meshrot", [hh_p, hh_b, hh_h])
    pos = vec("meshpos", [hh_x, hh_y, hh_z])
    mesh = gid("mesh")
    blocks = f'''        <VisualEffect.VisualEffect InstanceId="{guid}">
            <Name>{name}</Name>
            <EmitterList>
                <ObjectReference InstanceId="{mesh}" id="StaticMesh"/>
            </EmitterList>
        </VisualEffect.VisualEffect>
        <VisualEffect.StaticMesh InstanceId="{mesh}">
            <StaticMeshPosition>{pos}, 0.000000, 0.000000, 0.000000</StaticMeshPosition>
            <StaticMeshRotation>{rot}, -90.000000, 55.000000, 90.000000</StaticMeshRotation>
            <StaticMeshScale>{hh_scale}, 1.000000</StaticMeshScale>
            <Material>{HEAT_MATERIAL}</Material>
            <Mesh>{HEAT_MESH}</Mesh>
            <HeatDeformDeformation>{deform}, 2.500000, 1.750000</HeatDeformDeformation>
            <HeatDeformBlur>{hh_blur}, 1.000000</HeatDeformBlur>
        </VisualEffect.StaticMesh>
'''
    positions[guid] = (300, -100)
    positions[mesh] = (300, 0)
    return blocks


def write():
    global PREFIX
    for name, (guid, kind, material, local, cap) in EFFECTS.items():
        PREFIX = name + "/"
        nodes.clear(); positions.clear(); _y[0] = 0; _seed[0] = 0
        blocks = (build_heat(name, guid) if kind == "heat"
                  else build_particles(name, guid, material, local, cap, full=(kind in ("particles", "ring", "ribbon")),
                                       ring=(kind == "ring"), oriented=(material == RINGMAT),
                                       ribbon=(kind == "ribbon")))
        doc = ('﻿<?xml version="1.0" encoding="UTF-8"?>\n\n<SimBase.Document Type="AceXML" version="1,0">\n'
               "    <Descr>AceXML Document</Descr>\n    <WorldBase.Flight>\n" + blocks + "".join(nodes)
               + "    </WorldBase.Flight>\n</SimBase.Document>\n")
        ed = ('﻿<?xml version="1.0" encoding="UTF-8"?>\n\n'
              f'<SimBase.Document Type="EditionInfo" version="1,0" id="{name}_edition">\n'
              "    <Descr>This file contains useful additional properties for edition purpose.</Descr>\n"
              + "".join(f'    <SimEdition.EditionInfo RelatedInstanceId="{i}">\n        <NodePosition>{x:.3f},{y:.3f}</NodePosition>\n'
                        "    </SimEdition.EditionInfo>\n" for i, (x, y) in positions.items())
              + "</SimBase.Document>\n")
        for fn, text in ((f"{name}.xml", doc), (f"{name}_edition.xml", ed)):
            with open(os.path.join(OUT, fn), "w", encoding="utf-8", newline="\r\n") as fh:
                fh.write(text)
        print(f"{name}: {len(nodes)} nodes, guid {guid}")
    names = ["WingWater", "GenericMist", "GenericTrail", "GenericRibbon", "Marker"] + list(EFFECTS)
    lib = "\r\n".join(['<VisualEffectLibrary Version="1.0.0">'] + [f'\t<VisualEffect File="{n}"/>' for n in names]
                      + ["</VisualEffectLibrary>", ""])
    with open(os.path.join(OUT, "VisualEffectLibrary.vfxlib"), "w", encoding="utf-8", newline="") as fh:
        fh.write(lib)


if __name__ == "__main__":
    write()
