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
EFFECT = "{3D8F2A61-B47C-4E19-A5D2-8C61F0E3B7A9}"
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
bump  = op("Subtract", T, tt, (-900, 1000))                                          # t(1-t): 0 -> 0.25 -> 0
def gparam(name, default, pos): return node("GraphParameter", f"            <GraphParamName>{name}</GraphParamName>\n            <GraphParamDefaultValue>{Z}, {default:.6f}</GraphParamDefaultValue>\n", pos)
# GenericRibbon = GenericTrail drawn as a RIBBON (particles joined into a continuous camera-facing strip; gm_size = width). GenericTrail = GenericMist emitted in WORLD space: particles stay where they were born, so a moving aircraft leaves a trail (vortices).
# GenericRibbon: everything comes from spawn params (rainfx.ini), so one build serves many effects.
P = {}
for k, (name, dflt) in enumerate([("gm_dir_x", 0.0), ("gm_dir_y", 1.0), ("gm_dir_z", 0.0), ("gm_jitter", 0.5),
                                  ("gm_spread_x", 0.5), ("gm_spread_y", 0.2), ("gm_spread_z", 0.5),
                                  ("gm_size", 1.0), ("gm_life", 1.0), ("gm_alpha", 0.2), ("gm_rate", 160.0), ("gm_grow", 0.0), ("gm_grey", 1.0)]):
    P[name] = gparam(name, dflt, (-1700, 1100 + 90 * k))

def emitter(X, material, rate, cap):
    E, I, U, O = g(), g(), g(), g()
    def sr(axis_param, y):   # random -1..1 scaled by a param
        return op("Multiply", rand(-1.0, 1.0, (X-1100, y)), P[axis_param], (X-900, y))
    pos = vec3(sr("gm_spread_x", 0), sr("gm_spread_y", 90), sr("gm_spread_z", 180), (X-350, 90))
    def vj(dir_param, y):    # dir + random(-1..1) * jitter
        return op("Add", P[dir_param], op("Multiply", rand(-1.0, 1.0, (X-1300, y)), P["gm_jitter"], (X-1100, y)), (X-900, y))
    vel = vec3(vj("gm_dir_x", 300), vj("gm_dir_y", 390), vj("gm_dir_z", 480), (X-350, 390))
    lf   = op("Multiply", rand(0.7, 1.0, (X-550, 560)), P["gm_life"], (X-350, 560))
    size = op("Multiply", rand(0.6, 1.0, (X-700, 620)), P["gm_size"], (X-500, 620))
    grow = op("Add", op("Multiply", T, P["gm_grow"], (X-700, 700)), 1.0, (X-500, 700))   # 1 -> 1+grow over life
    usize = op("Multiply", size, grow, (X-350, 680))
    alpha = op("Multiply", op("Multiply", bump, 4.0, (X-550, 740)), P["gm_alpha"], (X-350, 740))
    col = vec4(P["gm_grey"], P["gm_grey"], P["gm_grey"], alpha, (X-150, 760))   # 1 = white mist, lower = grey smoke
    blocks.append(f'''        <VisualEffect.Emitter InstanceId="{E}">
            <Capacity>{cap}</Capacity>
            <ParticleRate>{P["gm_rate"]}, {rate:.6f}</ParticleRate>
            <EmitInLocalSpace>False</EmitInLocalSpace>
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
            <ParticleSize>
                <FloatIn>{usize}, 0.400000</FloatIn>
            </ParticleSize>
        </VisualEffect.BlockParticleUpdate>
        <VisualEffect.Output InstanceId="{O}">
            <Material>{material}</Material>
            <PrimitiveType>Ribbon</PrimitiveType>
            <ParticleColor>
                <ColorIn>{col}, 0, 0, 0, 0</ColorIn>
            </ParticleColor>
        </VisualEffect.Output>''')
    positions.extend([(E, (X, 40)), (I, (X+40, 380)), (U, (X+120, 560)), (O, (X+40, 680))])
    return E

main = emitter(300, MIST, rate=160, cap=3000)
emitters = [main]

xml = f'''﻿<?xml version="1.0" encoding="UTF-8"?>

<SimBase.Document Type="AceXML" version="1,0">
    <Descr>AceXML Document</Descr>
    <WorldBase.Flight>
        <VisualEffect.VisualEffect InstanceId="{EFFECT}">
            <Name>GenericRibbon</Name>
            <EmitterList>
{chr(10).join(f'                <ObjectReference InstanceId="{e}" id="Emitter"/>' for e in emitters)}
            </EmitterList>
        </VisualEffect.VisualEffect>
{chr(10).join(blocks)}
{chr(10).join(nodes)}
    </WorldBase.Flight>
</SimBase.Document>
'''
open(out + "/GenericRibbon.xml", "w", encoding="utf-8", newline="\r\n").write(xml)
ed = ['﻿<?xml version="1.0" encoding="UTF-8"?>', "",
      '<SimBase.Document Type="EditionInfo" version="1,0" id="GenericRibbon_edition">',
      "    <Descr>This file contains useful additional properties for edition purpose.</Descr>"]
for nid, (x, y) in [(EFFECT, (300, -100))] + positions:
    ed += [f'    <SimEdition.EditionInfo RelatedInstanceId="{nid}">', f"        <NodePosition>{x:.3f},{y:.3f}</NodePosition>", "    </SimEdition.EditionInfo>"]
ed += ["</SimBase.Document>", ""]
open(out + "/GenericRibbon_edition.xml", "w", encoding="utf-8", newline="\r\n").write("\n".join(ed))
print("ok", variant, len(nodes), "nodes")
