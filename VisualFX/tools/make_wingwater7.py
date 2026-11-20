# WingWater v19 (19 Nov): + ww_blue tint dial.
# WingWater v18: live-tunable. GraphParameter nodes ww_speed/ww_length/ww_width/ww_thick (multipliers, 1 = v17 look) sent from rainfx.ini.
# WingWater v4: soft spray that GROWS and FADES over each particle's life (what real smoke/spray effects do).
# Each particle: size grows from ~35% to 100% of its random max; opacity rises then falls (peak mid-life).
# Built only from node types whose saved format we've confirmed, plus Multiply/Subtract/Add (docs: xVariant/yVariant)
# and GetParticleAttribute AgeOverLifeTime (docs value list).
import uuid, sys
out = sys.argv[1]
variant = sys.argv[2] if len(sys.argv) > 2 else "mist"
Z = "{00000000-0000-0000-0000-000000000000}"
def g(): return "{" + str(uuid.uuid4()).upper() + "}"
EFFECT = "{FCE98D79-8557-4515-B3D5-B3389E95C39E}"
SPLASH = "{E344CBC7-6293-478C-983C-F5DA49C6D62C}"   # VFX_Water_Splash
MIST   = "{ABF127FE-7F13-45AF-A31B-9A6FFD02C00F}"   # VFX_Water_Mist
blocks, nodes, positions = [], [], []
seed = [1]
def node(kind, body, pos):
    nid, o = g(), g()
    nodes.append(f'        <VisualEffect.{kind} InstanceId="{nid}">\n            <OutputValue>{o}</OutputValue>\n{body}        </VisualEffect.{kind}>')
    positions.append((nid, pos)); return o
def v(x): return f"{x}, 0.000000" if isinstance(x, str) else f"{Z}, {x:.6f}"
PID = node("GetParticleAttribute", "            <ParticleAttributeType>id</ParticleAttributeType>\n", (-1400, 300))
T   = node("GetParticleAttribute", "            <ParticleAttributeType>AgeOverLifetime</ParticleAttributeType>\n", (-1400, 900))
def rand(lo, hi, pos):
    b = (f"            <MinRandValue>{Z}, {lo:.6f}</MinRandValue>\n            <MaxRandValue>{Z}, {hi:.6f}</MaxRandValue>\n"
         f"            <RandSeed>{Z}, {seed[0]*7919:.6f}</RandSeed>\n            <RandIndex>{PID}, {seed[0]:.6f}</RandIndex>\n")
    seed[0] += 1; return node("RandomValue", b, pos)
def op(kind, a, b, pos): return node(kind + "Operation", f"            <xVariant>{v(a)}</xVariant>\n            <yVariant>{v(b)}</yVariant>\n", pos)
def vec3(x, y, z, pos): return node("Vector3", f"            <x>{v(x)}</x>\n            <y>{v(y)}</y>\n            <z>{v(z)}</z>\n", pos)
def vec4(x, y, z, w, pos): return node("Vector4", f"            <x>{v(x)}</x>\n            <y>{v(y)}</y>\n            <z>{v(z)}</z>\n            <w>{v(w)}</w>\n", pos)

# Shared over-life curves (per particle, every frame):
# (size growth removed)
tt    = op("Multiply", T, T, (-1100, 1000))
bump  = op("Subtract", T, tt, (-900, 1000))
def gparam(name, pos): return node("GraphParameter", f"            <GraphParamName>{name}</GraphParamName>\n            <GraphParamDefaultValue>{Z}, 1.000000</GraphParamDefaultValue>\n", pos)
P_SPEED = gparam("ww_speed",  (-1400, 1200))
P_LEN   = gparam("ww_length", (-1400, 1300))
P_WIDTH = gparam("ww_width",  (-1400, 1400))
P_THICK = gparam("ww_thick",  (-1400, 1500))
P_BLUE  = node("GraphParameter", f"            <GraphParamName>ww_blue</GraphParamName>\n", (-1400, 1600))   # 0..1 blue tint (default 0)
NOTBLUE = op("Subtract", 1.0, P_BLUE, (-1200, 1600))                                          # t(1-t): 0 -> 0.25 -> 0

def emitter(X, material, rate, cap, maxsize, life, span, zstart, vel_aft, vel_up, peak_alpha):
    E, I, U, O = g(), g(), g(), g()
    pos = vec3(op("Multiply", rand(-span, span, (X-900, 0)), P_WIDTH, (X-700, 0)), rand(0.0, 0.2, (X-700, 90)), rand(zstart[0], zstart[1], (X-700, 180)), (X-350, 90))
    vel = vec3(rand(-0.6, 0.6, (X-700, 300)), rand(vel_up[0], vel_up[1], (X-700, 390)), op("Multiply", rand(-vel_aft[1], -vel_aft[0], (X-900, 480)), P_SPEED, (X-700, 480)), (X-350, 390))
    lf  = op("Multiply", rand(life[0], life[1], (X-550, 560)), P_LEN, (X-350, 560))
    smax = rand(maxsize[0], maxsize[1], (X-700, 620))       # same value every frame for a given particle (seeded by id)
    size = smax   # size over life disabled (Update-block size made the effect fail to spawn)
    alpha = op("Multiply", op("Multiply", bump, peak_alpha * 4.0, (X-550, 740)), P_THICK, (X-350, 740))
    col = vec4(NOTBLUE, NOTBLUE, 1.0, alpha, (X-150, 760))   # ww_blue tints it slightly blue
    blocks.append(f'''        <VisualEffect.Emitter InstanceId="{E}">
            <Capacity>{cap}</Capacity>
            <ParticleRate>{Z}, {rate:.6f}</ParticleRate>
            <EmitInLocalSpace>True</EmitInLocalSpace>
            <ParticleInit>
                <ObjectReference InstanceId="{I}" id="BlockParticleInit"/>
            </ParticleInit>
        </VisualEffect.Emitter>
        <VisualEffect.BlockParticleInit InstanceId="{I}">
            <ParticleUpdate>
                <ObjectReference InstanceId="{U}" id="BlockParticleUpdate"/>
            </ParticleUpdate>
            <ParticleLifetime>
                <FloatIn>{lf}, 1.000000</FloatIn>
            </ParticleLifetime>
            <ParticleSize>
                <FloatIn>{size}, 0.400000</FloatIn>
            </ParticleSize>
            <ParticlePosition>
                <Float3In>{pos}, 0.000000, 0.000000, 0.000000</Float3In>
            </ParticlePosition>
            <ParticleVelocity>
                <Float3In>{vel}, 0.000000, 0.000000, 0.000000</Float3In>
            </ParticleVelocity>
        </VisualEffect.BlockParticleInit>
        <VisualEffect.BlockParticleUpdate InstanceId="{U}">
            <ParticleOutput>
                <ObjectReference InstanceId="{O}" id="Output"/>
            </ParticleOutput>
        </VisualEffect.BlockParticleUpdate>
        <VisualEffect.Output InstanceId="{O}">
            <Material>{material}</Material>
            <ParticleColor>
                <ColorIn>{col}, 0, 0, 0, 0</ColorIn>
            </ParticleColor>
        </VisualEffect.Output>''')
    positions.extend([(E, (X, 40)), (I, (X+40, 380)), (U, (X+120, 560)), (O, (X+40, 680))])
    return E

# Spray starts just ahead of the leading edge above the engine, a bit wider than before, rolls up and streams back.
if variant == "splash":
    main = emitter(300, SPLASH, rate=120, cap=200, maxsize=(0.6, 1.2), life=(0.8, 1.3), span=3.6, zstart=(0.0, 0.8), vel_aft=(6.0, 9.0), vel_up=(0.5, 1.4), peak_alpha=0.30)
else:
    main = emitter(300, MIST,   rate=220, cap=700, maxsize=(0.7, 1.4), life=(0.55, 0.9), span=3.6, zstart=(0.0, 0.8), vel_aft=(11.0, 15.0), vel_up=(0.5, 1.4), peak_alpha=0.22)
emitters = [main]

xml = f'''﻿<?xml version="1.0" encoding="UTF-8"?>

<SimBase.Document Type="AceXML" version="1,0">
    <Descr>AceXML Document</Descr>
    <WorldBase.Flight>
        <VisualEffect.VisualEffect InstanceId="{EFFECT}">
            <Name>WingWater</Name>
            <EmitterList>
{chr(10).join(f'                <ObjectReference InstanceId="{e}" id="Emitter"/>' for e in emitters)}
            </EmitterList>
        </VisualEffect.VisualEffect>
{chr(10).join(blocks)}
{chr(10).join(nodes)}
    </WorldBase.Flight>
</SimBase.Document>
'''
open(out + "/WingWater.xml", "w", encoding="utf-8", newline="\r\n").write(xml)
ed = ['﻿<?xml version="1.0" encoding="UTF-8"?>', "",
      '<SimBase.Document Type="EditionInfo" version="1,0" id="WingWater_edition">',
      "    <Descr>This file contains useful additional properties for edition purpose.</Descr>"]
for nid, (x, y) in [(EFFECT, (300, -100))] + positions:
    ed += [f'    <SimEdition.EditionInfo RelatedInstanceId="{nid}">', f"        <NodePosition>{x:.3f},{y:.3f}</NodePosition>", "    </SimEdition.EditionInfo>"]
ed += ["</SimBase.Document>", ""]
open(out + "/WingWater_edition.xml", "w", encoding="utf-8", newline="\r\n").write("\n".join(ed))
print("ok", variant, len(nodes), "nodes")
