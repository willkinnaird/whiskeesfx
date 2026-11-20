// WhiskeesFX VisualFX - weather, water, vapour and vortex effects for MSFS 2024 (formerly GroundFeel RainFX).
//
// A standalone WASM module. Everything it shows is described in \work\visualfx.ini,
// which is re-read every 2 seconds, so effects can be added, moved and re-triggered
// while the sim is running. Each [effect.*] section says which built-in MSFS visual
// effect to spawn, where on the aircraft, and under which conditions.
// Status (what it sees, what's playing, recent events) goes to \work\status.txt.
//
// v12 (19 Nov 2026): up to 256 effects, 48 dials per effect, a 512 KB settings file, and
// [preset.NAME] sections: shared settings an effect pulls in with `preset = NAME`
// (the effect's own lines always win, whatever order they're in).

#include <MSFS/MSFS.h>
#include <MSFS/MSFS_Core.h>
#include <MSFS/MSFS_Vars.h>
#include <MSFS/MSFS_Vfx.h>

extern "C" {
typedef struct _IO_FILE FILE;
FILE* fopen(const char* path, const char* mode);
unsigned long fread(void* ptr, unsigned long size, unsigned long n, FILE* f);
unsigned long fwrite(const void* ptr, unsigned long size, unsigned long n, FILE* f);
int fclose(FILE* f);
double strtod(const char* s, char** end);
int snprintf(char* buf, unsigned long n, const char* fmt, ...);
}

// ------------------------------------------------------------------ small helpers
static double simTime = 0;
static char evts[30][200];
static int evtNext = 0, evtCount = 0;
#define LOG(...) do { char _b[180]; snprintf(_b, sizeof(_b), __VA_ARGS__); \
	snprintf(evts[evtNext], sizeof(evts[0]), "%8.1fs  %s\n", simTime, _b); \
	evtNext = (evtNext + 1) % 30; if (evtCount < 30) evtCount++; } while (0)

static bool streq(const char* a, const char* b) { while (*a && *a == *b) { a++; b++; } return *a == *b; }
static void scopy(char* d, const char* s, int n) { int i = 0; for (; s[i] && i < n - 1; i++) d[i] = s[i]; d[i] = 0; }
static char lower(char c) { return c >= 'A' && c <= 'Z' ? c + 32 : c; }
static bool containsNoCase(const char* hay, const char* needle) {
	if (!*needle) return false;
	for (; *hay; hay++) {
		int i = 0;
		while (needle[i] && lower(hay[i]) == lower(needle[i])) i++;
		if (!needle[i]) return true;
	}
	return false;
}
static void lastIniReset();
static char title[256] = "";
static bool isSpace(char c) { return c == ' ' || c == '\t' || c == '\r'; }
static double clampd(double v, double lo, double hi) { return v < lo ? lo : v > hi ? hi : v; }

// ------------------------------------------------------------------ sim state
struct Sim {
	double surfCond = 0, precipRate = 0, agl = 0, gs = 0, ias = 0;
	int surfType = 0, precip = 0, nEng = 0;
	bool onGround = false, raining = false, hardSurface = false, simWet = false;
	bool inCloud = false, humid = false;
	double visibility = 99999;
	double n1[4] = {0}, rev[4] = {0};
	double wetness = 0;       // our own runway-water model, 0..1
	bool wet = false;          // final "wet" decision
	bool touchdown = false;    // true for a short moment after the mains touch down
	double touchdownTimer = 0;
	bool wasOnGround = true;
	double airTime = 0;
};
static Sim sim;

static FsUnitId uBool, uFeet, uEnum, uKnots, uPercent, uNumber, uMmh, uMeters;
static FsUnitId uString;
static FsAVarId vTitle;
static double titleTimer = 0;
static void readTitle() {
	char buf[256] = {0};
	FsVarParamArray arr;
	FsVarError err = fsVarsAVarBufferGet(vTitle, uString, arr, buf, sizeof(buf) - 1, FS_OBJECT_ID_USER_AIRCRAFT);
	buf[255] = 0;
	if (err != FS_VAR_ERROR_NONE) return;
	if (!streq(buf, title)) {
		scopy(title, buf, sizeof(title));
		lastIniReset();
		LOG("aircraft: %s", title);
	}
}
static FsAVarId vOnGround, vAgl, vSurfCond, vSurfType, vGs, vIas, vN1, vRev, vPrecipState, vPrecipRate, vNumEng, vInCloud, vVis;

static double aget(FsAVarId id, FsUnitId unit, int index = -1) {
	double r = 0;
	FsVarParamVariant v;
	FsVarParamArray arr;
	if (index >= 0) {
		v.type = FsVarParamTypeInteger;
		v.intValue = (unsigned int)index;
		arr.size = 1;
		arr.array = &v;
	}
	fsVarsAVarGet(id, unit, arr, &r, FS_OBJECT_ID_USER_AIRCRAFT);
	return r;
}

// ------------------------------------------------------------------ settings
struct Global {
	double enabled = 1;
	double rainCountsAsWet = 1;
	double wetBuildMinutes = 3;   // how long heavy rain takes to make the runway fully wet
	double dryMinutes = 30;       // how long it takes to dry after rain stops
	double wetThreshold = 0.15;   // wetness above which effects count as "wet"
	double engineX = 5.75, engineY = -1.6, engineZ = 1.0;   // engine position (m from reference point), x mirrored
	double mainX = 3.8, mainZ = 0.0, noseZ = 16.9, groundY = -3.0;
	double touchdownSeconds = 1.5;
	// Outer engines on four-engine aircraft (A380, 747). Engines 1 and 4 use these, 2 and 3 use engine_x/y/z.
	double outerX = 0, outerY = -999, outerZ = -999;
	double humidForce = -1;       // -1 = work it out, 0 = never humid, 1 = always humid
	double humidVisibility = 3000; // visibility below this (m) counts as humid (haze, mist, fog)
	double angleScale = 1;        // multiply rotate values by this (1 = degrees passed as-is; 0.0174533 if the sim wants radians)
};
static Global G;

enum Anchor { A_REF, A_ENGINES, A_MAINS, A_NOSE, A_WHEELS, A_INNER, A_OUTER, A_SIDES };
enum CondVar { C_N1, C_REV, C_GS, C_IAS, C_AGL, C_WETNESS, C_V1, C_V2, C_V3, C_V4, C_V5, C_V6, C_V7, C_V8 };
// User-defined sim variables: var1 = AMBIENT TEMPERATURE, Celsius   (optional :index after the name)
struct UserVar { char spec[96] = {0}; FsAVarId id = 0; FsUnitId unit = 0; int index = -1; bool ok = false; bool lvar = false; char lname[96] = {0}; double value = 0; };
static UserVar uvars[8];
// lvar_search = word1, word2 ...  -> status.txt lists every L:var whose name contains one of the words.
static char lvarSearch[128] = "";

struct Cond {
	int kind;       // 0 = flag, 1 = comparison
	int flag;       // index into flag list
	CondVar var;
	bool greater;
	double value;
};

enum Flag { F_WET, F_DRY, F_RAINING, F_GROUND, F_AIR, F_TOUCHDOWN, F_ALWAYS, F_NEVER, F_HUMID };

struct EffectDef {
	char name[48];
	char guid[48];
	Anchor anchor = A_REF;
	double off[3] = {0, 0, 0};
	Cond conds[8];
	int nConds = 0;
	bool engineParams = false;   // pass engineIndex/engineNumber graph params
	bool on = true;
	double rot[3] = {0, 0, 0};   // pitch bank heading, mirrored on the right-hand side
	int pOff[48];                 // extra graph params: arena offsets of "name\0expression\0"  ({e} engine, {i} inboard sign)
	int nParams = 0;
	double restart = 0;           // restart = N: after switching off, let particles live N s, then drop the instance so the next start is fresh (bursts)
	unsigned setMask = 0;         // which keys this section set itself (presets fill only the rest)
	unsigned sig = 0;            // changes when spawn-time settings change, forcing a respawn
};
enum { S_GUID = 1, S_AT = 2, S_OFFSET = 4, S_ROTATE = 8, S_WHEN = 16, S_ON = 32, S_EP = 64 };

// v12 limits: 256 effects, 64 presets, 48 dials per effect, 512 KB settings file.
static const int MAX_DEFS = 256;
static const int MAX_PRESETS = 64;
static const int MAX_PARAMS = 48;
static const int PEXPR_MAX = 300;
static EffectDef defs[MAX_DEFS];
static int nDefs = 0;
static EffectDef presets[MAX_PRESETS];
static int nPresets = 0;
static char arena[786432];         // every param name + expression lives here once (presets are shared, not copied)
static int arenaUsed = 0;
static const char* pNameOf(const EffectDef& d, int k) { return arena + d.pOff[k]; }
static const char* pExprOf(const EffectDef& d, int k) { const char* s = arena + d.pOff[k]; while (*s) s++; return s + 1; }
static void setParam(EffectDef& d, const char* name, int off) {
	for (int k = 0; k < d.nParams; k++) if (streq(pNameOf(d, k), name)) { d.pOff[k] = off; return; }   // later line wins
	if (d.nParams < MAX_PARAMS) d.pOff[d.nParams++] = off;
	else LOG("effect %s: more than %d params, '%s' ignored", d.name, MAX_PARAMS, name);
}
static void applyPreset(EffectDef& d, const EffectDef& pr) {
	// Fills in everything the section hasn't set itself; params already set in the section stay.
	if (!(d.setMask & S_GUID)) scopy(d.guid, pr.guid, sizeof(d.guid));
	if (!(d.setMask & S_AT) && (pr.setMask & S_AT)) d.anchor = pr.anchor;
	if (!(d.setMask & S_EP)) d.engineParams = d.engineParams || pr.engineParams;
	if (!(d.setMask & S_OFFSET) && (pr.setMask & S_OFFSET)) for (int i = 0; i < 3; i++) d.off[i] = pr.off[i];
	if (!(d.setMask & S_ROTATE) && (pr.setMask & S_ROTATE)) for (int i = 0; i < 3; i++) d.rot[i] = pr.rot[i];
	if (!(d.setMask & S_WHEN) && (pr.setMask & S_WHEN)) { d.nConds = pr.nConds; for (int i = 0; i < pr.nConds; i++) d.conds[i] = pr.conds[i]; }
	if (!(d.setMask & S_ON) && (pr.setMask & S_ON)) d.on = pr.on;
	if (d.restart <= 0) d.restart = pr.restart;
	for (int k = 0; k < pr.nParams; k++) {
		const char* n = pNameOf(pr, k);
		bool have = false;
		for (int j = 0; j < d.nParams; j++) if (streq(pNameOf(d, j), n)) { have = true; break; }
		if (!have) setParam(d, n, pr.pOff[k]);
	}
	d.setMask |= pr.setMask & (S_GUID | S_AT | S_OFFSET | S_ROTATE | S_WHEN | S_ON);
}

static bool parseCond(const char* tok, Cond& c) {
	struct { const char* n; Flag f; } flags[] = {
		{"wet", F_WET}, {"dry", F_DRY}, {"raining", F_RAINING}, {"humid", F_HUMID}, {"ground", F_GROUND}, {"air", F_AIR},
		{"touchdown", F_TOUCHDOWN}, {"always", F_ALWAYS}, {"never", F_NEVER},
	};
	for (auto& f : flags) if (streq(tok, f.n)) { c.kind = 0; c.flag = f.f; return true; }
	struct { const char* n; CondVar v; } vars[] = {
		{"n1", C_N1}, {"reverser", C_REV}, {"rev", C_REV}, {"gs", C_GS}, {"speed", C_GS}, {"ias", C_IAS},
		{"agl", C_AGL}, {"wetness", C_WETNESS},
		{"v1", C_V1}, {"v2", C_V2}, {"v3", C_V3}, {"v4", C_V4}, {"v5", C_V5}, {"v6", C_V6}, {"v7", C_V7}, {"v8", C_V8},
	};
	for (auto& v : vars) {
		int i = 0;
		while (v.n[i] && tok[i] == v.n[i]) i++;
		if (v.n[i] == 0 && (tok[i] == '>' || tok[i] == '<')) {
			c.kind = 1;
			c.var = v.v;
			c.greater = tok[i] == '>';
			char* end = nullptr;
			c.value = strtod(tok + i + 1, &end);
			return end != tok + i + 1;
		}
	}
	return false;
}

static void parseVec(const char* s, double* out) {
	char* end = nullptr;
	for (int i = 0; i < 3; i++) {
		while (isSpace(*s) || *s == ',') s++;
		double v = strtod(s, &end);
		if (end == s) return;
		out[i] = v;
		s = end;
	}
}

static bool profileActive = true;   // false while inside an [aircraft.*] section that doesn't match
static char profileName[64] = "";

static void setGlobal(const char* key, const char* val) {
	if (!profileActive) return;
	if (key[0] == 'v' && key[1] == 'a' && key[2] == 'r' && key[3] >= '1' && key[3] <= '8' && !key[4]) {
		scopy(uvars[key[3] - '1'].spec, val, sizeof(uvars[0].spec));
		return;
	}
	if (streq(key, "lvar_search")) { scopy(lvarSearch, val, sizeof(lvarSearch)); return; }
	struct { const char* k; double* v; } keys[] = {
		{"enabled", &G.enabled}, {"rain_counts_as_wet", &G.rainCountsAsWet},
		{"humid_force", &G.humidForce}, {"humid_visibility", &G.humidVisibility},
		{"wet_build_minutes", &G.wetBuildMinutes}, {"dry_minutes", &G.dryMinutes}, {"wet_threshold", &G.wetThreshold},
		{"engine_x", &G.engineX}, {"engine_y", &G.engineY}, {"engine_z", &G.engineZ},
		{"main_gear_x", &G.mainX}, {"main_gear_z", &G.mainZ}, {"nose_gear_z", &G.noseZ}, {"ground_y", &G.groundY},
		{"touchdown_seconds", &G.touchdownSeconds},
		{"engine_outer_x", &G.outerX}, {"engine_outer_y", &G.outerY}, {"engine_outer_z", &G.outerZ},
		{"angle_scale", &G.angleScale},
	};
	for (auto& k : keys) if (streq(key, k.k)) { char* e; double v = strtod(val, &e); if (e != val) *k.v = v; }
}

static void setEffect(EffectDef& d, const char* key, const char* val) {
	if (streq(key, "guid")) { scopy(d.guid, val, sizeof(d.guid)); d.setMask |= S_GUID; }
	else if (streq(key, "on")) { d.on = strtod(val, nullptr) > 0; d.setMask |= S_ON; }
	else if (streq(key, "offset")) { parseVec(val, d.off); d.setMask |= S_OFFSET; }
	else if (streq(key, "engine_params")) { d.engineParams = strtod(val, nullptr) > 0; d.setMask |= S_EP; }
	else if (streq(key, "rotate")) { parseVec(val, d.rot); d.setMask |= S_ROTATE; }
	else if (streq(key, "restart")) d.restart = strtod(val, nullptr);
	else if (streq(key, "preset")) {
		// preset = NAME [NAME2 ...]: fill in from [preset.NAME] sections defined ABOVE this one (first name wins)
		const char* p = val;
		while (*p) {
			while (isSpace(*p) || *p == ',') p++;
			char nm[48]; int n = 0;
			while (*p && !isSpace(*p) && *p != ',' && n < 47) nm[n++] = *p++;
			nm[n] = 0;
			if (!n) break;
			bool found = false;
			for (int i = 0; i < nPresets; i++) if (streq(presets[i].name, nm)) { applyPreset(d, presets[i]); found = true; break; }
			if (!found) LOG("%s: preset '%s' not found (presets must come before the effects that use them)", d.name, nm);
		}
	}
	else if (streq(key, "param")) {
		const char* p = val; char nm[32]; int n = 0;
		while (*p && !isSpace(*p) && n < 31) nm[n++] = *p++;
		nm[n] = 0;
		while (*p && !isSpace(*p)) p++;
		while (isSpace(*p)) p++;
		int len = 0; while (p[len]) len++;
		if (len > PEXPR_MAX) { LOG("%s: param %s longer than %d characters, cut", d.name, nm, PEXPR_MAX); len = PEXPR_MAX; }
		if (!n || !len) { LOG("effect %s: param needs a name and an expression", d.name); return; }
		if (arenaUsed + n + len + 2 > (int)sizeof(arena)) { LOG("settings too big: param %s ignored", nm); return; }
		int off = arenaUsed;
		for (int i = 0; i < n; i++) arena[arenaUsed++] = nm[i];
		arena[arenaUsed++] = 0;
		for (int i = 0; i < len; i++) arena[arenaUsed++] = p[i];
		arena[arenaUsed++] = 0;
		setParam(d, nm, off);
	}
	else if (streq(key, "at")) {
		d.setMask |= S_AT;
		if (streq(val, "engines")) { d.anchor = A_ENGINES; d.engineParams = true; }
		else if (streq(val, "mains")) d.anchor = A_MAINS;
		else if (streq(val, "nose")) d.anchor = A_NOSE;
		else if (streq(val, "wheels")) d.anchor = A_WHEELS;
		else if (streq(val, "inner_engines")) { d.anchor = A_INNER; d.engineParams = true; }
		else if (streq(val, "outer_engines")) { d.anchor = A_OUTER; d.engineParams = true; }
		else if (streq(val, "sides")) d.anchor = A_SIDES;   // left + right copy, offset x = distance out from the centreline
		else d.anchor = A_REF;
	} else if (streq(key, "when")) {
		d.setMask |= S_WHEN;
		d.nConds = 0;
		char tok[32];
		const char* p = val;
		while (*p && d.nConds < 8) {
			while (isSpace(*p) || *p == ',') p++;
			int n = 0;
			while (*p && !isSpace(*p) && *p != ',' && n < 31) tok[n++] = *p++;
			tok[n] = 0;
			if (n == 0) break;
			Cond c;
			if (parseCond(tok, c)) d.conds[d.nConds++] = c;
			else LOG("effect %s: didn't understand condition '%s'", d.name, tok);
		}
	}
}

static void parseIni(const char* text) {
	Global fresh;
	G = fresh;
	nDefs = 0;
	nPresets = 0;
	arenaUsed = 0;
	for (auto& u : uvars) u.spec[0] = 0;
	lvarSearch[0] = 0;
	profileActive = true;
	profileName[0] = 0;
	EffectDef* cur = nullptr;
	const char* p = text;
	static char line[1024];
	while (*p) {
		int n = 0;
		while (*p && *p != '\n') { if (n < 1023) line[n++] = *p; p++; }
		if (*p == '\n') p++;
		line[n] = 0;
		char* s = line;
		while (isSpace(*s)) s++;
		char* e = s + 0; while (*e) e++;
		while (e > s && isSpace(e[-1])) *--e = 0;
		if (!*s || *s == '#' || *s == ';') continue;
		if (*s == '[') {
			char* close = s; while (*close && *close != ']') close++;
			*close = 0;
			const char* name = s + 1;
			if (name[0] == 'e' && name[1] == 'f' && name[2] == 'f' && name[3] == 'e' && name[4] == 'c' && name[5] == 't' && name[6] == '.') {
				if (nDefs < MAX_DEFS) {
					EffectDef blank;
					defs[nDefs] = blank;
					cur = &defs[nDefs++];
					scopy(cur->name, name + 7, sizeof(cur->name));
				} else { cur = nullptr; LOG("more than %d effects: [effect.%s] ignored", MAX_DEFS, name + 7); }
				profileActive = true;
			} else if (name[0] == 'p' && name[1] == 'r' && name[2] == 'e' && name[3] == 's' && name[4] == 'e' && name[5] == 't' && name[6] == '.') {
				// [preset.NAME]: shared settings (guid, at, when, params...) that effects pull in with preset = NAME
				if (nPresets < MAX_PRESETS) {
					EffectDef blank;
					presets[nPresets] = blank;
					cur = &presets[nPresets++];
					scopy(cur->name, name + 7, sizeof(cur->name));
				} else { cur = nullptr; LOG("more than %d presets", MAX_PRESETS); }
				profileActive = true;
			} else if (name[0] == 'a' && containsNoCase(name, "aircraft.")) {
				// [aircraft.fenix a321] - the following settings apply only if the loaded aircraft's title contains that text.
				cur = nullptr;
				profileActive = containsNoCase(title, name + 9);
				if (profileActive && !profileName[0]) scopy(profileName, name + 9, sizeof(profileName));
			} else { cur = nullptr; profileActive = true; }
			continue;
		}
		char* eq = s; while (*eq && *eq != '=') eq++;
		if (!*eq) continue;
		char* ke = eq; while (ke > s && isSpace(ke[-1])) ke--;
		*ke = 0;
		char* v = eq + 1; while (isSpace(*v)) v++;
		char* hash = v; while (*hash && *hash != '#') hash++;
		while (hash > v && isSpace(hash[-1])) hash--;
		*hash = 0;
		if (cur) setEffect(*cur, s, v); else setGlobal(s, v);
	}
	// Spawn-time signature (guid, params, engine params) so edits respawn the effect.
	for (int i = 0; i < nDefs; i++) {
		EffectDef& d = defs[i];
		unsigned h = 2166136261u;
		auto mix = [&](const char* t) { for (; *t; t++) h = (h ^ (unsigned char)*t) * 16777619u; h = (h ^ 0xff) * 16777619u; };
		mix(d.guid);
		for (int k = 0; k < d.nParams; k++) { mix(pNameOf(d, k)); mix(pExprOf(d, k)); }
		h ^= d.engineParams ? 0x5bd1e995u : 0;
		d.sig = h;
	}
	// Resolve user variables.
	for (int i = 0; i < 8; i++) {
		UserVar& u = uvars[i];
		u.ok = false; u.index = -1; u.value = 0;
		if (!u.spec[0]) continue;
		char name[96]; scopy(name, u.spec, sizeof(name));
		char* comma = name; while (*comma && *comma != ',') comma++;
		const char* unit = "Number";
		if (*comma) { *comma = 0; unit = comma + 1; while (isSpace(*unit)) unit++; }
		char* e = comma; while (e > name && isSpace(e[-1])) *--e = 0;
		u.lvar = (name[0] == 'L' || name[0] == 'l') && name[1] == ':';
		if (u.lvar) {
			const char* ln = name + 2; while (isSpace(*ln)) ln++;
			scopy(u.lname, ln, sizeof(u.lname));
			u.unit = fsVarsGetUnitId(unit);
			u.id = fsVarsGetLVarId(u.lname);
			u.ok = u.id != FS_VAR_INVALID_ID;
			if (!u.ok) LOG("var%d: L:%s not found yet (will keep trying)", i + 1, u.lname);
			continue;
		}
		char* colon = name; while (*colon && *colon != ':') colon++;
		if (*colon) { *colon = 0; u.index = (int)strtod(colon + 1, nullptr); }
		u.id = fsVarsGetAVarId(name);
		u.unit = fsVarsGetUnitId(unit);
		u.ok = u.id != (FsAVarId)-1;
		if (!u.ok) LOG("var%d: sim doesn't know '%s'", i + 1, name);
	}
	if (G.outerY < -900) G.outerY = G.engineY;
	if (G.outerZ < -900) G.outerZ = G.engineZ;
}

static char lastIni[524288];
static void lastIniReset() { lastIni[0] = 0; }
static void loadIni() {
	FILE* f = fopen("\\work\\visualfx.ini", "rb");
	if (!f) return;
	static char buf[524288];
	unsigned long n = fread(buf, 1, sizeof(buf) - 1, f);
	fclose(f);
	buf[n] = 0;
	if (streq(buf, lastIni)) return;
	scopy(lastIni, buf, sizeof(lastIni));
	parseIni(buf);
	LOG("settings loaded: %d effects, %d presets, %d KB of dials%s%s", nDefs, nPresets, arenaUsed / 1024, profileName[0] ? ", aircraft profile: " : "", profileName);
}

// ------------------------------------------------------------------ effect instances
// Each definition can have up to 3 instances (left/right engine, left/right/nose wheel).
struct Inst {
	FsVfxId id = FSVFXID_NULL;
	bool playing = false;
	FsVec3d off = {0, 0, 0};
	char guid[48] = {0};
	unsigned sig = 0;
	FsVec3d rot = {0, 0, 0};
	double retry = 0;
	double kill = 0;      // restart countdown after switching off
	int failures = 0;
};
static Inst inst[MAX_DEFS][4];
static char instName[MAX_DEFS][48];   // which definition each row was last used for

static void stopInst(Inst& in, bool destroy) {
	if (in.id != FSVFXID_NULL) {
		if (in.playing) fsVfxStopInstance(in.id);
		if (destroy) { fsVfxDestroyInstance(in.id); in.id = FSVFXID_NULL; }
	}
	in.playing = false;
}

static double condValue(CondVar v, int engine) {
	switch (v) {
	case C_N1: case C_REV: {
		const double* a = v == C_N1 ? sim.n1 : sim.rev;
		if (engine >= 0) return a[engine];
		double m = 0; for (int e = 0; e < 4; e++) if (a[e] > m) m = a[e];
		return m;
	}
	case C_GS: return sim.gs;
	case C_IAS: return sim.ias;
	case C_AGL: return sim.agl;
	case C_WETNESS: return sim.wetness * 100;
	default: return uvars[v - C_V1].value;
	}
	return 0;
}

static bool evalConds(const EffectDef& d, int engine) {
	for (int i = 0; i < d.nConds; i++) {
		const Cond& c = d.conds[i];
		bool ok = true;
		if (c.kind == 0) {
			switch (c.flag) {
			case F_WET: ok = sim.wet; break;
			case F_DRY: ok = !sim.wet; break;
			case F_RAINING: ok = sim.raining; break;
			case F_HUMID: ok = sim.humid; break;
			case F_GROUND: ok = sim.onGround; break;
			case F_AIR: ok = !sim.onGround; break;
			case F_TOUCHDOWN: ok = sim.touchdown; break;
			case F_ALWAYS: ok = true; break;
			case F_NEVER: ok = false; break;
			}
		} else {
			double v = condValue(c.var, engine);
			ok = c.greater ? v > c.value : v < c.value;
		}
		if (!ok) return false;
	}
	return true;
}

static void driveInst(int di, int k, Inst& in, const EffectDef& d, bool want, FsVec3d off, int engine, double dt, bool mirror) {
	if (in.id != FSVFXID_NULL && !fsVfxIsValid(in.id)) { in.id = FSVFXID_NULL; in.playing = false; }
	if (in.id != FSVFXID_NULL && in.sig != d.sig) stopInst(in, true); // effect/params changed in settings
	double sc = G.angleScale;
	FsVec3d pbh = {d.rot[0] * sc, d.rot[1] * sc * (mirror ? -1 : 1), d.rot[2] * sc * (mirror ? -1 : 1)};
	if (!want) {
		if (in.playing) {
			fsVfxStopInstance(in.id); in.playing = false; LOG("%s[%d] off", d.name, k);
			if (d.restart > 0) in.kill = d.restart;
		} else if (in.kill > 0 && in.id != FSVFXID_NULL) {
			in.kill -= dt;
			if (in.kill <= 0) stopInst(in, true);
		}
		return;
	}
	if (in.kill > 0 && !in.playing && in.id != FSVFXID_NULL) { stopInst(in, true); in.kill = 0; }   // start fresh
	if (in.id == FSVFXID_NULL) {
		if (in.retry > 0) { in.retry -= dt; return; }
		FsVfxGraphParam params[MAX_PARAMS + 2];
		static char exprs[MAX_PARAMS][PEXPR_MAX + 64];
		char idx[4] = {(char)('1' + (engine < 0 ? 0 : engine)), 0};
		int np = 0;
		if (d.engineParams) {
			params[0].paramName = "engineIndex";
			params[0].RPNExpression = idx;
			params[1].paramName = "engineNumber";
			params[1].RPNExpression = "(A:NUMBER OF ENGINES, Number)";
			np = 2;
		}
		for (int q = 0; q < d.nParams; q++) {
			// substitute {e} with this instance's engine number (1-based)
			const char* src = pExprOf(d, q); char* dst = exprs[q]; int n = 0;
			while (*src && n < PEXPR_MAX + 56) {
				if (src[0] == '{' && src[1] == 'e' && src[2] == '}') { dst[n++] = idx[0]; src += 3; }
				else if (src[0] == '{' && src[1] == 'i' && src[2] == '}') { if (off.x > 0) dst[n++] = '-'; dst[n++] = '1'; src += 3; } // inboard sign: +x is the aircraft's left
				else dst[n++] = *src++;
			}
			dst[n] = 0;
			params[np].paramName = pNameOf(d, q);
			params[np].RPNExpression = exprs[q];
			np++;
		}
		in.id = fsVfxSpawnOnSimObject(d.guid, SIM_OBJECT_ID_USER, nullptr, off, pbh, -1.f, np ? params : nullptr, np);
		if (in.id == FSVFXID_NULL) {
			in.failures++;
			in.retry = in.failures < 5 ? 2.0 : 30.0;
			LOG("%s[%d] spawn FAILED (guid %s) - retrying", d.name, k, d.guid);
			return;
		}
		in.failures = 0;
		scopy(in.guid, d.guid, sizeof(in.guid));
		in.sig = d.sig;
		in.rot = pbh;
		in.off = off;
		in.playing = true;
		LOG("%s[%d] on at (%.2f, %.2f, %.2f)", d.name, k, off.x, off.y, off.z);
		return;
	}
	double dx = in.off.x - off.x, dy = in.off.y - off.y, dz = in.off.z - off.z;
	if (dx * dx + dy * dy + dz * dz > 1e-6) { fsVfxSetOffset(in.id, off); in.off = off; }
	double rx = in.rot.x - pbh.x, ry = in.rot.y - pbh.y, rz = in.rot.z - pbh.z;
	if (rx * rx + ry * ry + rz * rz > 1e-9) { fsVfxSetRotation(in.id, pbh); in.rot = pbh; LOG("%s[%d] rotated", d.name, k); }
	if (!in.playing) { fsVfxPlayInstance(in.id); in.playing = true; LOG("%s[%d] on", d.name, k); }
}

static void stopAll(bool destroy) {
	for (int i = 0; i < MAX_DEFS; i++) for (int k = 0; k < 4; k++) stopInst(inst[i][k], destroy);
}

// ------------------------------------------------------------------ sim reading + wetness model
static void readSim(double dt) {
	sim.surfCond = aget(vSurfCond, uEnum);
	sim.surfType = (int)aget(vSurfType, uEnum);
	sim.precip = (int)aget(vPrecipState, uNumber);
	sim.precipRate = aget(vPrecipRate, uMmh);
	sim.raining = (sim.precip & 4) != 0;
	sim.inCloud = aget(vInCloud, uBool) > 0.5;
	sim.visibility = aget(vVis, uMeters);
	// MSFS has no humidity or dew point variable, so: rain, cloud or poor visibility count as humid.
	sim.humid = G.humidForce >= 0 ? G.humidForce > 0
	          : (sim.raining || (sim.precip & 8) || sim.inCloud || (sim.visibility > 0 && sim.visibility < G.humidVisibility));   // v10: falling snow counts too
	int t = sim.surfType;
	sim.hardSurface = t == 0 || t == 4 || t == 15 || t == 17 || t == 19 || t == 23;
	sim.simWet = sim.surfCond == 1;
	sim.agl = aget(vAgl, uFeet);
	sim.onGround = aget(vOnGround, uBool) > 0;
	sim.gs = aget(vGs, uKnots);
	sim.ias = aget(vIas, uKnots);
	sim.nEng = (int)aget(vNumEng, uNumber);
	static double lvarRetry = 0;
	lvarRetry -= dt;
	for (int i = 0; i < 8; i++) {
		UserVar& u = uvars[i];
		if (!u.spec[0]) continue;
		if (u.lvar) {
			if (!u.ok && lvarRetry <= 0) {
				u.id = fsVarsGetLVarId(u.lname);
				u.ok = u.id != FS_VAR_INVALID_ID;
				if (u.ok) LOG("var%d: found L:%s", i + 1, u.lname);
			}
			double r = 0;
			if (u.ok && fsVarsLVarGet(u.id, u.unit, &r) == FS_VAR_ERROR_NONE) u.value = r;
		} else if (u.ok) u.value = aget(u.id, u.unit, u.index);
	}
	if (lvarRetry <= 0) lvarRetry = 2.0;
	for (int e = 0; e < 4; e++) {
		sim.n1[e] = e < sim.nEng ? aget(vN1, uPercent, e + 1) : 0;
		sim.rev[e] = e < sim.nEng ? aget(vRev, uPercent, e + 1) : 0;
	}

	// Runway water model: builds while it rains (faster in heavier rain), dries slowly afterwards.
	double rate = sim.raining ? clampd(sim.precipRate > 0 ? sim.precipRate / 8.0 : 0.5, 0.2, 2.0) : 0;
	if (sim.simWet) rate = rate < 0.5 ? 0.5 : rate;
	if (rate > 0) sim.wetness += dt / (60.0 * (G.wetBuildMinutes > 0.05 ? G.wetBuildMinutes : 0.05)) * rate;
	else sim.wetness -= dt / (60.0 * (G.dryMinutes > 0.05 ? G.dryMinutes : 0.05));
	sim.wetness = clampd(sim.wetness, 0, 1);
	sim.wet = sim.hardSurface && (sim.simWet || sim.wetness > G.wetThreshold || (G.rainCountsAsWet > 0 && sim.raining));

	// Touchdown detection.
	if (!sim.onGround) sim.airTime += dt;
	if (sim.onGround && !sim.wasOnGround && sim.airTime > 2) {
		sim.touchdownTimer = G.touchdownSeconds;
		LOG("touchdown at %.0f kt (wet=%d)", sim.gs, sim.wet);
	}
	if (sim.onGround) sim.airTime = 0;
	sim.wasOnGround = sim.onGround;
	if (sim.touchdownTimer > 0) sim.touchdownTimer -= dt;
	sim.touchdown = sim.touchdownTimer > 0;
}

// ------------------------------------------------------------------ status file
static unsigned long frames = 0;
static const char* anchorName(Anchor a) {
	switch (a) { case A_ENGINES: return "engines"; case A_MAINS: return "mains"; case A_NOSE: return "nose"; case A_WHEELS: return "wheels"; case A_INNER: return "inner"; case A_OUTER: return "outer"; case A_SIDES: return "sides"; default: return "ref"; }
}
static void writeStatus() {
	static char buf[98304];
	int n = snprintf(buf, sizeof(buf),
		"WhiskeesFX VisualFX status (rewritten every second)\n"
		"frames=%lu  time=%.1fs  enabled=%.0f\n"
		"wet=%d  runway water=%.0f%%  sim says wet=%d (surface condition %.0f)  raining=%d (precip %d, rate %.1f)  surface type %d (hard=%d)\n"
		"humid=%d  (in cloud=%d, visibility %.0f m, humid_force %.0f)\n"
		"on ground=%d  agl=%.1f ft  gs=%.1f kt  ias=%.1f kt  touchdown=%d  engines=%d\n"
		"N1 %.1f / %.1f / %.1f / %.1f %%   reverser %.1f / %.1f / %.1f / %.1f %%\n"
		"aircraft title: %s\nprofile in use: %s\n",
		frames, simTime, G.enabled, sim.wet, sim.wetness * 100, sim.simWet, sim.surfCond, sim.raining, sim.precip, sim.precipRate,
		sim.surfType, sim.hardSurface, sim.humid, sim.inCloud, sim.visibility, G.humidForce, sim.onGround, sim.agl, sim.gs, sim.ias, sim.touchdown, sim.nEng,
		sim.n1[0], sim.n1[1], sim.n1[2], sim.n1[3], sim.rev[0], sim.rev[1], sim.rev[2], sim.rev[3],
		title[0] ? title : "(not read)", profileName[0] ? profileName : "(none - using the general settings)");
	for (int i = 0; i < 8; i++) if (uvars[i].spec[0])
		n += snprintf(buf + n, sizeof(buf) - n, "v%d = %.3f   (%s)%s\n", i + 1, uvars[i].value, uvars[i].spec, uvars[i].ok ? "" : "  NOT FOUND");
	if (lvarSearch[0]) {
		// Split the search words.
		char words[8][32]; int nw = 0;
		const char* s = lvarSearch;
		while (*s && nw < 8) {
			while (*s == ',' || isSpace(*s)) s++;
			int k = 0;
			while (*s && *s != ',' && !isSpace(*s)) { if (k < 31) words[nw][k++] = *s; s++; }
			words[nw][k] = 0;
			if (k) nw++;
		}
		n += snprintf(buf + n, sizeof(buf) - n, "\nL:vars matching '%s':\n", lvarSearch);
		int shown = 0, misses = 0;
		for (int id = 0; id < 30000 && misses < 500 && shown < 80 && n < (int)sizeof(buf) - 2000; id++) {
			char nm[128];
			if (fsVarsGetLVarName(id, nm, sizeof(nm)) != FS_VAR_ERROR_NONE || !nm[0]) { misses++; continue; }
			misses = 0;
			bool hit = false;
			for (int w = 0; w < nw && !hit; w++) hit = containsNoCase(nm, words[w]);
			if (!hit) continue;
			double r = 0;
			fsVarsLVarGet(id, uNumber, &r);
			n += snprintf(buf + n, sizeof(buf) - n, "  %-48s %.3f\n", nm, r);
			shown++;
		}
		if (!shown) n += snprintf(buf + n, sizeof(buf) - n, "  (none)\n");
	}
	n += snprintf(buf + n, sizeof(buf) - n, "\neffects:\n");
	for (int i = 0; i < nDefs && n < (int)sizeof(buf) - 400; i++) {
		const EffectDef& d = defs[i];
		n += snprintf(buf + n, sizeof(buf) - n, "  %-20s at=%-7s %s ", d.name, anchorName(d.anchor), d.on ? "" : "(switched off)");
		for (int k = 0; k < 4; k++) {
			const Inst& in = inst[i][k];
			if (in.id == FSVFXID_NULL && in.failures == 0) continue;
			n += snprintf(buf + n, sizeof(buf) - n, " [%d %s (%.1f,%.1f,%.1f)]", k,
				in.id == FSVFXID_NULL ? "FAILED" : in.playing ? "PLAYING" : "stopped", in.off.x, in.off.y, in.off.z);
		}
		n += snprintf(buf + n, sizeof(buf) - n, "\n");
	}
	n += snprintf(buf + n, sizeof(buf) - n, "\nrecent events (oldest first):\n");
	for (int i = 0; i < evtCount && n < (int)sizeof(buf) - 210; i++) {
		int k = (evtNext - evtCount + i + 30) % 30;
		n += snprintf(buf + n, sizeof(buf) - n, "%s", evts[k]);
	}
	FILE* f = fopen("\\work\\status.txt", "wb");
	if (f) { fwrite(buf, 1, (unsigned long)n, f); fclose(f); }
}

// ------------------------------------------------------------------ module callbacks
static double iniTimer = 0, fileTimer = 0;

extern "C" MSFS_CALLBACK void module_init(void) {
	uBool = fsVarsGetUnitId("Bool");
	uFeet = fsVarsGetUnitId("Feet");
	uEnum = fsVarsGetUnitId("Enum");
	uKnots = fsVarsGetUnitId("Knots");
	uPercent = fsVarsGetUnitId("Percent");
	uNumber = fsVarsGetUnitId("Number");
	uMmh = fsVarsGetUnitId("Millimeters of water");
	vOnGround = fsVarsGetAVarId("SIM ON GROUND");
	vAgl = fsVarsGetAVarId("PLANE ALT ABOVE GROUND MINUS CG");
	vSurfCond = fsVarsGetAVarId("SURFACE CONDITION");
	vSurfType = fsVarsGetAVarId("SURFACE TYPE");
	vGs = fsVarsGetAVarId("GROUND VELOCITY");
	vIas = fsVarsGetAVarId("AIRSPEED INDICATED");
	vN1 = fsVarsGetAVarId("TURB ENG N1");
	vRev = fsVarsGetAVarId("TURB ENG REVERSE NOZZLE PERCENT");
	vPrecipState = fsVarsGetAVarId("AMBIENT PRECIP STATE");
	vPrecipRate = fsVarsGetAVarId("AMBIENT PRECIP RATE");
	vInCloud = fsVarsGetAVarId("AMBIENT IN CLOUD");
	vVis = fsVarsGetAVarId("AMBIENT VISIBILITY");
	uBool = fsVarsGetUnitId("Bool");
	uMeters = fsVarsGetUnitId("Meters");
	vNumEng = fsVarsGetAVarId("NUMBER OF ENGINES");
	uString = fsVarsGetUnitId("String");
	vTitle = fsVarsGetAVarId("TITLE");
	readTitle();
	LOG("module loaded");
	loadIni();
	writeStatus();
}

extern "C" MSFS_CALLBACK void module_deinit(void) { stopAll(true); }

extern "C" MSFS_CALLBACK void module_late_update(float dTime) {
	double dt = dTime > 0 && dTime < 1 ? dTime : 0.016;
	simTime += dt;
	frames++;
	iniTimer -= dt;
	titleTimer -= dt;
	if (titleTimer <= 0) { titleTimer = 5.0; readTitle(); }
	if (iniTimer <= 0) { iniTimer = 2.0; loadIni(); }
	fileTimer -= dt;
	if (fileTimer <= 0) { fileTimer = 1.0; writeStatus(); }

	readSim(dt);

	// Definitions may have been reordered/removed: free rows that now hold a different effect.
	for (int i = 0; i < MAX_DEFS; i++) {
		const char* want = i < nDefs ? defs[i].name : "";
		if (!streq(instName[i], want)) {
			for (int k = 0; k < 4; k++) stopInst(inst[i][k], true);
			scopy(instName[i], want, sizeof(instName[i]));
		}
	}
	if (G.enabled <= 0) { stopAll(false); return; }

	for (int i = 0; i < nDefs; i++) {
		const EffectDef& d = defs[i];
		const double* o = d.off;
		switch (d.anchor) {
		case A_REF:
			driveInst(i, 0, inst[i][0], d, d.on && evalConds(d, -1), FsVec3d{o[0], o[1], o[2]}, -1, dt, false);
			break;
		case A_ENGINES:
		case A_INNER:
		case A_OUTER: {
			bool four = sim.nEng >= 4;
			for (int e = 0; e < 4; e++) {
				bool outer = four && (e == 0 || e == 3);
				bool exists = four || e < 2;
				if (d.anchor == A_INNER && outer) exists = false;
				if (d.anchor == A_OUTER && !outer) exists = false;
				bool left = four ? e < 2 : e == 0;
				double side = left ? -1 : 1;
				double ex = outer ? G.outerX : G.engineX, ey = outer ? G.outerY : G.engineY, ez = outer ? G.outerZ : G.engineZ;
				FsVec3d p = {side * (ex + o[0]), ey + o[1], ez + o[2]};
				driveInst(i, e, inst[i][e], d, exists && d.on && sim.nEng > e && evalConds(d, e), p, e, dt, !left);
			}
			break;
		}
		case A_MAINS:
		case A_WHEELS:
			for (int w = 0; w < 2; w++) {
				double side = w == 0 ? -1 : 1;
				FsVec3d p = {side * (G.mainX + o[0]), G.groundY + o[1], G.mainZ + o[2]};
				driveInst(i, w, inst[i][w], d, d.on && evalConds(d, -1), p, -1, dt, w != 0);
			}
			if (d.anchor == A_MAINS) break;
			goto nose_wheel;   // wheels = mains + nose (v10 fix: A_SIDES had been inserted into this fall-through)
		case A_SIDES:
			for (int w = 0; w < 2; w++) {
				double side = w == 0 ? -1 : 1;
				FsVec3d p = {side * o[0], o[1], o[2]};
				driveInst(i, w, inst[i][w], d, d.on && evalConds(d, -1), p, -1, dt, w != 0);
			}
			break;
		case A_NOSE: nose_wheel: {
			FsVec3d p = {o[0], G.groundY + o[1], G.noseZ + o[2]};
			driveInst(i, 2, inst[i][2], d, d.on && evalConds(d, -1), p, -1, dt, false);
			break;
		}
		}
	}
}
