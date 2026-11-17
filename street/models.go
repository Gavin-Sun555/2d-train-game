package street

import "github.com/Gavin-Sun555/2d-train-game/internal/locale"

// NoseKind shapes the cab ends of a tram model when drawn from above.
type NoseKind uint8

const (
	NosePanoramic   NoseKind = iota // rounded glass bow (Citadis 302, Inekon, Kumamoto, Haizhu)
	NoseSleek                       // long tapered aero nose with light blades (Citadis X05, HU300, Suzhou)
	NoseFaceted                     // multi-bevelled front (Avenio, Flexity Outlook, Sirius, Optics Valley)
	NoseStreamlined                 // sculpted LRV bow with bumper (Siemens S700, Flexity 2, Xijiao, PCC)
	NoseAngular                     // crisp flat-visor face (Stadler TINA, Brookville Liberty)
	NoseWedge                       // bold dark-apron wedge (Skoda ForCity, P3010, Sifang)
	NoseBoxy                        // clean cube/vestibule cab (Hiroden 5200, Centram, classic steel trams)
)

// RoofKind selects how rooftop equipment and shrouds are arranged.
type RoofKind uint8

const (
	RoofShrouded    RoofKind = iota // full-length aerodynamic side fairings
	RoofModular                     // modular HVAC & brake resistor pods
	RoofLRV                         // heavy-duty LRV twin HVAC pods & conduits
	RoofBattery                     // off-wire supercap / ESS battery packs + compact AC
	RoofCompact                     // clean flat roof with central AC unit
	RoofClerestory                  // pre-1950s raised lantern / clerestory monitor roof
	RoofVintageArch                 // pre-1950s arched canvas / slat roof with ventilators
)

// PantoKind selects the current collector geometry on the roof.
type PantoKind uint8

const (
	PantoSingleArm   PantoKind = iota // modern Z-type single-arm pantograph
	PantoDiamond                      // classic diamond / lozenge pantograph
	PantoTrolleyPole                  // vintage swivelling trolley pole with collector wheel/shoe
)

// TramModel describes a real-world modern or historic tram / streetcar / LRV:
// its performance limits (acceleration, deceleration and supported top speed),
// body width, era and visual styling.
type TramModel struct {
	Key             string
	zh, en          string
	shortZh         string
	shortEn         string
	regionZh        string
	regionEn        string
	MaxSpeedKmh     int     // maximum speed supported by the vehicle itself (km/h)
	Accel           float64 // maximum service acceleration (m/s^2)
	Decel           float64 // comfortable service deceleration (m/s^2)
	Width           float64 // body width (m)
	Nose            NoseKind
	Roof            RoofKind
	Panto           PantoKind
	DarkRoof        bool
	TopLamp         bool // centre upper headlamp on the cab brow
	SingleHeadlamp  bool // single central dash headlamp (pre-1950s streetcars)
	Vintage         bool // 1950s and earlier classic tram
	Accent          [3]uint8
	PanoramicWindow bool // extra-wide side glass bays
}

func (m TramModel) Name() string      { return locale.T(m.zh, m.en) }
func (m TramModel) ShortName() string { return locale.T(m.shortZh, m.shortEn) }
func (m TramModel) Region() string    { return locale.T(m.regionZh, m.regionEn) }

// TramModels is the catalogue of 31 trams: 21 modern trams (Europe, North
// America, Japan and China) and 10 classic trams from the 1950s and earlier.
var TramModels = []TramModel{
	// --- Modern: Europe (6) ---
	{
		Key: "citadis-302",
		zh:  "阿尔斯通 Citadis 302", en: "Alstom Citadis 302",
		shortZh: "Citadis 302", shortEn: "Citadis 302",
		regionZh: "欧洲", regionEn: "Europe",
		MaxSpeedKmh: 70, Accel: 1.20, Decel: 1.35, Width: 2.40,
		Nose: NosePanoramic, Roof: RoofShrouded, Panto: PantoSingleArm,
		Accent: [3]uint8{236, 240, 244},
	},
	{
		Key: "citadis-x05",
		zh:  "阿尔斯通 Citadis X05", en: "Alstom Citadis X05",
		shortZh: "Citadis X05", shortEn: "Citadis X05",
		regionZh: "欧洲", regionEn: "Europe",
		MaxSpeedKmh: 80, Accel: 1.30, Decel: 1.40, Width: 2.40,
		Nose: NoseSleek, Roof: RoofShrouded, Panto: PantoSingleArm,
		DarkRoof: true, PanoramicWindow: true,
		Accent: [3]uint8{42, 48, 56},
	},
	{
		Key: "siemens-avenio",
		zh:  "西门子 Avenio", en: "Siemens Avenio",
		shortZh: "西门子 Avenio", shortEn: "Siemens Avenio",
		regionZh: "欧洲", regionEn: "Europe",
		MaxSpeedKmh: 80, Accel: 1.25, Decel: 1.40, Width: 2.65,
		Nose: NoseFaceted, Roof: RoofModular, Panto: PantoSingleArm,
		Accent: [3]uint8{214, 222, 230},
	},
	{
		Key: "flexity-2",
		zh:  "庞巴迪 Flexity 2", en: "Bombardier Flexity 2",
		shortZh: "Flexity 2", shortEn: "Flexity 2",
		regionZh: "欧洲", regionEn: "Europe",
		MaxSpeedKmh: 70, Accel: 1.15, Decel: 1.30, Width: 2.65,
		Nose: NoseStreamlined, Roof: RoofShrouded, Panto: PantoSingleArm,
		Accent: [3]uint8{242, 244, 246},
	},
	{
		Key: "stadler-tina",
		zh:  "施泰德 TINA", en: "Stadler TINA",
		shortZh: "施泰德 TINA", shortEn: "Stadler TINA",
		regionZh: "欧洲", regionEn: "Europe",
		MaxSpeedKmh: 80, Accel: 1.35, Decel: 1.45, Width: 2.40,
		Nose: NoseAngular, Roof: RoofModular, Panto: PantoSingleArm,
		PanoramicWindow: true,
		Accent:          [3]uint8{228, 232, 236},
	},
	{
		Key: "skoda-forcity",
		zh:  "斯柯达 ForCity Smart", en: "Skoda ForCity Smart",
		shortZh: "ForCity Smart", shortEn: "ForCity Smart",
		regionZh: "欧洲", regionEn: "Europe",
		MaxSpeedKmh: 65, Accel: 1.40, Decel: 1.45, Width: 2.46,
		Nose: NoseWedge, Roof: RoofModular, Panto: PantoSingleArm,
		DarkRoof: true,
		Accent:   [3]uint8{34, 38, 44},
	},

	// --- Modern: North America (5) ---
	{
		Key: "siemens-s700",
		zh:  "西门子 S700 轻轨", en: "Siemens S700 LRV",
		shortZh: "西门子 S700", shortEn: "Siemens S700",
		regionZh: "北美", regionEn: "N. America",
		MaxSpeedKmh: 90, Accel: 1.30, Decel: 1.45, Width: 2.65,
		Nose: NoseStreamlined, Roof: RoofLRV, Panto: PantoSingleArm,
		TopLamp: true,
		Accent:  [3]uint8{216, 224, 232},
	},
	{
		Key: "flexity-outlook",
		zh:  "庞巴迪 Flexity Outlook", en: "Flexity Outlook",
		shortZh: "Flexity Outlook", shortEn: "Flexity Outlook",
		regionZh: "北美", regionEn: "N. America",
		MaxSpeedKmh: 70, Accel: 1.05, Decel: 1.25, Width: 2.54,
		Nose: NoseFaceted, Roof: RoofLRV, Panto: PantoSingleArm,
		TopLamp: true,
		Accent:  [3]uint8{246, 246, 248},
	},
	{
		Key: "brookville-liberty",
		zh:  "布鲁克维尔 Liberty", en: "Brookville Liberty",
		shortZh: "Liberty 现代电车", shortEn: "Brookville Liberty",
		regionZh: "北美", regionEn: "N. America",
		MaxSpeedKmh: 70, Accel: 1.15, Decel: 1.30, Width: 2.46,
		Nose: NoseAngular, Roof: RoofBattery, Panto: PantoSingleArm,
		TopLamp: true,
		Accent:  [3]uint8{62, 70, 80},
	},
	{
		Key: "inekon-trio",
		zh:  "伊内孔 12 Trio", en: "Inekon 12 Trio",
		shortZh: "Inekon 12 Trio", shortEn: "Inekon 12 Trio",
		regionZh: "北美", regionEn: "N. America",
		MaxSpeedKmh: 70, Accel: 1.20, Decel: 1.30, Width: 2.46,
		Nose: NosePanoramic, Roof: RoofCompact, Panto: PantoSingleArm,
		TopLamp: true,
		Accent:  [3]uint8{226, 230, 236},
	},
	{
		Key: "kinkisharyo-p3010",
		zh:  "近畿车辆 P3010 轻轨", en: "Kinkisharyo P3010 LRV",
		shortZh: "近畿 P3010", shortEn: "Kinkisharyo P3010",
		regionZh: "北美", regionEn: "N. America",
		MaxSpeedKmh: 95, Accel: 1.35, Decel: 1.50, Width: 2.65,
		Nose: NoseWedge, Roof: RoofLRV, Panto: PantoSingleArm,
		TopLamp: true,
		Accent:  [3]uint8{204, 212, 220},
	},

	// --- Modern: Japan (5) ---
	{
		Key: "hu300-lightline",
		zh:  "宇都宫 HU300 雷都线", en: "Utsunomiya HU300 Lightline",
		shortZh: "宇都宫 HU300", shortEn: "HU300 Lightline",
		regionZh: "日本", regionEn: "Japan",
		MaxSpeedKmh: 70, Accel: 1.25, Decel: 1.35, Width: 2.65,
		Nose: NoseSleek, Roof: RoofShrouded, Panto: PantoSingleArm,
		DarkRoof: true, PanoramicWindow: true,
		Accent: [3]uint8{244, 200, 42},
	},
	{
		Key: "hiroden-5200",
		zh:  "广岛电铁 5200形 APEX", en: "Hiroden 5200 APEX",
		shortZh: "广电 5200 APEX", shortEn: "Hiroden 5200",
		regionZh: "日本", regionEn: "Japan",
		MaxSpeedKmh: 60, Accel: 1.10, Decel: 1.30, Width: 2.45,
		Nose: NoseBoxy, Roof: RoofBattery, Panto: PantoSingleArm,
		TopLamp: true,
		Accent:  [3]uint8{236, 242, 238},
	},
	{
		Key: "toyama-centram",
		zh:  "富山 9000形 Centram", en: "Toyama 9000 Centram",
		shortZh: "富山 Centram", shortEn: "Toyama Centram",
		regionZh: "日本", regionEn: "Japan",
		MaxSpeedKmh: 50, Accel: 1.00, Decel: 1.25, Width: 2.40,
		Nose: NoseBoxy, Roof: RoofCompact, Panto: PantoSingleArm,
		DarkRoof: true, PanoramicWindow: true,
		Accent: [3]uint8{230, 234, 240},
	},
	{
		Key: "kumamoto-0800",
		zh:  "熊本市电 0800形", en: "Kumamoto 0800 Cocoro",
		shortZh: "熊本 0800形", shortEn: "Kumamoto 0800",
		regionZh: "日本", regionEn: "Japan",
		MaxSpeedKmh: 50, Accel: 0.95, Decel: 1.20, Width: 2.40,
		Nose: NosePanoramic, Roof: RoofCompact, Panto: PantoDiamond,
		Accent: [3]uint8{218, 194, 152},
	},
	{
		Key: "sapporo-sirius",
		zh:  "札幌市电 1100形 Sirius", en: "Sapporo 1100 Sirius",
		shortZh: "札幌 1100 Sirius", shortEn: "Sapporo 1100",
		regionZh: "日本", regionEn: "Japan",
		MaxSpeedKmh: 55, Accel: 1.15, Decel: 1.30, Width: 2.30,
		Nose: NoseFaceted, Roof: RoofCompact, Panto: PantoSingleArm,
		TopLamp: true,
		Accent:  [3]uint8{242, 246, 250},
	},

	// --- Modern: China (5) ---
	{
		Key: "crrc-haizhu",
		zh:  "中车株机 海珠有轨电车", en: "CRRC Haizhu Supercap Tram",
		shortZh: "中车 海珠电车", shortEn: "CRRC Haizhu",
		regionZh: "中国", regionEn: "China",
		MaxSpeedKmh: 70, Accel: 1.20, Decel: 1.35, Width: 2.65,
		Nose: NosePanoramic, Roof: RoofBattery, Panto: PantoSingleArm,
		Accent: [3]uint8{244, 244, 246},
	},
	{
		Key: "crrc-suzhou",
		zh:  "中车长客 苏州高新电车", en: "CRRC Suzhou Low-Floor Tram",
		shortZh: "中车 苏州电车", shortEn: "CRRC Suzhou",
		regionZh: "中国", regionEn: "China",
		MaxSpeedKmh: 70, Accel: 1.25, Decel: 1.35, Width: 2.65,
		Nose: NoseSleek, Roof: RoofShrouded, Panto: PantoSingleArm,
		PanoramicWindow: true,
		Accent:          [3]uint8{102, 204, 162},
	},
	{
		Key: "crrc-sifang",
		zh:  "中车四方 城阳有轨电车", en: "CRRC Sifang Chengyang Tram",
		shortZh: "中车 四方电车", shortEn: "CRRC Sifang",
		regionZh: "中国", regionEn: "China",
		MaxSpeedKmh: 70, Accel: 1.30, Decel: 1.40, Width: 2.65,
		Nose: NoseWedge, Roof: RoofModular, Panto: PantoSingleArm,
		DarkRoof: true,
		Accent:   [3]uint8{44, 52, 64},
	},
	{
		Key: "crrc-xijiao",
		zh:  "中车大连 北京西郊线", en: "CRRC Beijing Xijiao Tram",
		shortZh: "北京西郊线电车", shortEn: "CRRC Xijiao",
		regionZh: "中国", regionEn: "China",
		MaxSpeedKmh: 80, Accel: 1.25, Decel: 1.40, Width: 2.65,
		Nose: NoseStreamlined, Roof: RoofLRV, Panto: PantoSingleArm,
		TopLamp: true,
		Accent:  [3]uint8{238, 238, 240},
	},
	{
		Key: "crrc-optics-valley",
		zh:  "中车浦镇 武汉光谷电车", en: "CRRC Wuhan Optics Valley",
		shortZh: "武汉光谷光子号", shortEn: "CRRC Optics Valley",
		regionZh: "中国", regionEn: "China",
		MaxSpeedKmh: 70, Accel: 1.15, Decel: 1.30, Width: 2.65,
		Nose: NoseFaceted, Roof: RoofBattery, Panto: PantoSingleArm,
		PanoramicWindow: true,
		Accent:          [3]uint8{246, 248, 250},
	},

	// --- Classic / Pre-1950s Vintage Streetcars (10) ---
	{
		Key: "pcc-1936",
		zh:  "PCC 流线型电车 (1936)", en: "PCC Streamliner (1936)",
		shortZh: "PCC 电车 1936", shortEn: "PCC (1936)",
		regionZh: "北美·老式", regionEn: "N. America · 1936",
		MaxSpeedKmh: 65, Accel: 1.35, Decel: 1.35, Width: 2.54,
		Nose: NoseStreamlined, Roof: RoofVintageArch, Panto: PantoTrolleyPole,
		Vintage: true, SingleHeadlamp: true,
		Accent: [3]uint8{246, 236, 208},
	},
	{
		Key: "birney-1916",
		zh:  "伯尼安全型电车 (1916)", en: "Birney Safety Car (1916)",
		shortZh: "伯尼电车 1916", shortEn: "Birney (1916)",
		regionZh: "北美·老式", regionEn: "N. America · 1916",
		MaxSpeedKmh: 40, Accel: 0.75, Decel: 1.00, Width: 2.36,
		Nose: NosePanoramic, Roof: RoofClerestory, Panto: PantoTrolleyPole,
		Vintage: true, SingleHeadlamp: true,
		Accent: [3]uint8{242, 228, 194},
	},
	{
		Key: "peter-witt-1928",
		zh:  "彼得·威特型电车 (1928)", en: "Peter Witt Streetcar (1928)",
		shortZh: "彼得·威特 1928", shortEn: "Peter Witt (1928)",
		regionZh: "欧美·老式", regionEn: "Milan/US · 1928",
		MaxSpeedKmh: 50, Accel: 0.90, Decel: 1.10, Width: 2.35,
		Nose: NoseFaceted, Roof: RoofClerestory, Panto: PantoTrolleyPole,
		Vintage: true, SingleHeadlamp: true,
		Accent: [3]uint8{246, 232, 198},
	},
	{
		Key: "perley-thomas-1923",
		zh:  "新奥尔良 900型 (1923)", en: "Perley Thomas 900 (1923)",
		shortZh: "新奥尔良 900型", shortEn: "Perley Thomas 1923",
		regionZh: "北美·老式", regionEn: "N. America · 1923",
		MaxSpeedKmh: 45, Accel: 0.80, Decel: 1.05, Width: 2.44,
		Nose: NosePanoramic, Roof: RoofClerestory, Panto: PantoTrolleyPole,
		Vintage: true, SingleHeadlamp: true, DarkRoof: true,
		Accent: [3]uint8{238, 222, 186},
	},
	{
		Key: "lisbon-1930",
		zh:  "里斯本 山城老电车 (1930)", en: "Lisbon Remodelado (1930)",
		shortZh: "里斯本老电车", shortEn: "Lisbon (1930)",
		regionZh: "欧洲·老式", regionEn: "Europe · 1930",
		MaxSpeedKmh: 45, Accel: 0.95, Decel: 1.20, Width: 2.25,
		Nose: NosePanoramic, Roof: RoofClerestory, Panto: PantoTrolleyPole,
		Vintage: true, SingleHeadlamp: true,
		Accent: [3]uint8{250, 244, 226},
	},
	{
		Key: "vienna-type-m-1928",
		zh:  "维也纳 M型老电车 (1928)", en: "Vienna Type M Tram (1928)",
		shortZh: "维也纳 M型 1928", shortEn: "Vienna M (1928)",
		regionZh: "欧洲·老式", regionEn: "Europe · 1928",
		MaxSpeedKmh: 50, Accel: 0.85, Decel: 1.10, Width: 2.24,
		Nose: NoseBoxy, Roof: RoofClerestory, Panto: PantoDiamond,
		Vintage: true, SingleHeadlamp: true,
		Accent: [3]uint8{246, 238, 214},
	},
	{
		Key: "london-feltham-1931",
		zh:  "伦敦 Feltham 电车 (1931)", en: "London Feltham Tram (1931)",
		shortZh: "伦敦 Feltham", shortEn: "Feltham (1931)",
		regionZh: "欧洲·老式", regionEn: "Europe · 1931",
		MaxSpeedKmh: 55, Accel: 0.95, Decel: 1.15, Width: 2.28,
		Nose: NoseFaceted, Roof: RoofVintageArch, Panto: PantoTrolleyPole,
		Vintage: true, SingleHeadlamp: true,
		Accent: [3]uint8{244, 236, 212},
	},
	{
		Key: "berlin-t24-1924",
		zh:  "柏林 T24型有轨电车 (1924)", en: "Berlin T24 Classic (1924)",
		shortZh: "柏林 T24 (1924)", shortEn: "Berlin T24 (1924)",
		regionZh: "欧洲·老式", regionEn: "Europe · 1924",
		MaxSpeedKmh: 50, Accel: 0.85, Decel: 1.10, Width: 2.20,
		Nose: NoseBoxy, Roof: RoofVintageArch, Panto: PantoDiamond,
		Vintage: true, SingleHeadlamp: true, DarkRoof: true,
		Accent: [3]uint8{242, 232, 198},
	},
	{
		Key: "dalian-3000-1935",
		zh:  "大连 3000型老电车 (1935)", en: "Dalian 3000 Classic (1935)",
		shortZh: "大连老电车 1935", shortEn: "Dalian 3000 (1935)",
		regionZh: "中国·老式", regionEn: "China · 1935",
		MaxSpeedKmh: 45, Accel: 0.80, Decel: 1.05, Width: 2.40,
		Nose: NosePanoramic, Roof: RoofClerestory, Panto: PantoDiamond,
		Vintage: true, SingleHeadlamp: true, DarkRoof: true,
		Accent: [3]uint8{244, 234, 204},
	},
	{
		Key: "hiroden-650-1942",
		zh:  "广岛电铁 650形 (1942)", en: "Hiroden 650 Classic (1942)",
		shortZh: "广电 650形 1942", shortEn: "Hiroden 650 (1942)",
		regionZh: "日本·老式", regionEn: "Japan · 1942",
		MaxSpeedKmh: 45, Accel: 0.80, Decel: 1.10, Width: 2.40,
		Nose: NoseBoxy, Roof: RoofVintageArch, Panto: PantoDiamond,
		Vintage: true, SingleHeadlamp: true, TopLamp: true, DarkRoof: true,
		Accent: [3]uint8{238, 228, 196},
	},
}

// Operating speed limits set per line (km/h), bounded above by the line's
// chosen tram model's MaxSpeedKmh.
const (
	MinOperatingSpeedKmh  = 20
	OperatingSpeedStepKmh = 5
)

// DefaultTramModel is the rolling stock a new line starts with.
func DefaultTramModel() TramModel { return TramModels[0] }

// TramModelIndex finds a model by key, returning 0 (the default model) when
// key is empty or unknown.
func TramModelIndex(key string) int {
	for i, m := range TramModels {
		if m.Key == key {
			return i
		}
	}
	return 0
}

// ModelIndex is the index in TramModels of the line's chosen rolling stock.
func (l *Line) ModelIndex() int { return TramModelIndex(l.Model) }

// TramModel is the rolling stock running on the line.
func (l *Line) TramModel() TramModel { return TramModels[l.ModelIndex()] }

// OperatingSpeedKmh is the line's configured operating top speed in km/h,
// never exceeding the vehicle's own supported top speed.
func (l *Line) OperatingSpeedKmh() int {
	m := l.TramModel()
	if l.MaxSpeed <= 0 {
		return m.MaxSpeedKmh
	}
	return min(max(l.MaxSpeed, MinOperatingSpeedKmh), m.MaxSpeedKmh)
}

// OperatingSpeed is the line's operating top speed in m/s.
func (l *Line) OperatingSpeed() float64 { return float64(l.OperatingSpeedKmh()) / 3.6 }
