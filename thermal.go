package thermal

import (
	"bytes"
	"fmt"
	"io"
	"math"
	"text/tabwriter"

	"github.com/Konstantin8105/pow"
)

// Material is typical interface with thermal conductivity between 2 temperatires
type Material interface {
	ConductivityAvg(F1, F2 float64) float64
}

// MaterialPolynomial is material with polynomial thermal conductivity property
// by:
//	f[0] + f[1]*T + f[2]*T*T + ...
//
type MaterialPolynomial struct {
	factors []float64
}

// NewMaterialPolynominal return material with polynomial thermal
// conductivity property
func NewMaterialPolynomial(c ...float64) Material {
	return MaterialPolynomial{factors: c}
}

// ConductivityAvg return thermal conductivity between 2 temperatires.
// Temperature unit: degree F.
func (m MaterialPolynomial) ConductivityAvg(F2, F1 float64) float64 {
	K := make([]float64, len(m.factors))
	for i := range m.factors {
		K[i] = m.factors[i]
	}
	for i := 0; i < len(m.factors); i++ {
		K[i] *= (pow.En(F2, i+1) - pow.En(F1, i+1)) / float64(i+1)
	}
	Ksum := 0.0
	for i := range K {
		Ksum += K[i]
	}
	Ksum = Ksum / (F2 - F1)
	return Ksum
}

// MaterialExp is material with exponents function thermal conductivity by:
//	ln(k) = a + b * T
type MaterialExp struct {
	a, b float64
}

// NewMaterialExp return material with exponents functions
func NewMaterialExp(a, b float64) Material {
	return MaterialExp{a: a, b: b}
}

// ConductivityAvg return thermal conductivity between 2 temperatires.
// Temperature unit: degree F.
func (m MaterialExp) ConductivityAvg(F1, F2 float64) float64 {
	return 1.0 / (F2 - F1) * (math.Exp(m.a+m.b*F2) - math.Exp(m.a+m.b*F1)) / m.b
}

// MaterialType3 is material type 3
type MaterialType3 struct {
	a1, b1, TL float64
	a2, b2, TU float64
	a3, b3     float64
}

// NewMaterialType3 return material type 3
func NewMaterialType3(
	a1, b1, TL float64,
	a2, b2, TU float64,
	a3, b3 float64,
) Material {
	return MaterialType3{
		a1: a1, b1: b1, TL: TL,
		a2: a2, b2: b2, TU: TU,
		a3: a3, b3: b3,
	}
}

// ConductivityAvg return thermal conductivity between 2 temperatires.
// Temperature unit: degree F.
func (m MaterialType3) ConductivityAvg(F1, F2 float64) float64 {
	Kf := func(F float64) float64 {
		if F <= m.TL {
			return m.a1 + m.b1*F
		}
		if F <= m.TU {
			return m.a2 + m.b2*F
		}
		return m.a3 + m.b3*F
	}

	var (
		amount = 100
		K      float64
		dF     = (F2 - F1) / float64(amount-1)
	)
	Ki := make([]float64, amount)
	for i := 0; i < amount; i++ {
		Ki[i] = Kf(F1 + float64(i)*dF)
	}
	for i := 0; i < amount-1; i++ {
		K += (Ki[i] + Ki[i+1]) / 2.0 * dF
	}
	K = K / (F2 - F1)
	return K
}

// Layer of insulation
type Layer struct {
	Thk float64
	Mat Material
}

// ExternalSurface is property of thermal surface
type ExternalSurface struct {
	isSurf bool
	surf   float64
	wind   float64
	emiss  float64
	orient Orientation
}

func Surf(Surf float64) *ExternalSurface {
	es := new(ExternalSurface)
	es.surf = Surf
	es.isSurf = true
	return es
}

func Emiss(Wind, Emiss float64, orient Orientation) *ExternalSurface {
	es := new(ExternalSurface)
	es.wind = Wind
	es.emiss = Emiss
	es.orient = orient
	return es
}

func (es *ExternalSurface) surcof(Dia, Ts, Tamb float64, isCylinder bool) {
	Tair := (Tamb+Ts)/2.0 + 459.69
	ATdelt := math.Abs(Tamb - Ts)
	if ATdelt < 1.0 {
		ATdelt = 1.0
	}

	var Dx float64
	var coef float64

	if isCylinder {
		Dx = Dia * 12.0
		switch es.orient {
		case 1:
			coef = 1.016
		case 2:
			coef = 1.235
		}
		if 24 < Dx {
			Dx = 24.0
		}
	} else {
		Dx = 24.0
		switch es.orient {
		case 1:
			coef = 1.394
		case 2:
			coef = 0.89
		case 3:
			coef = 1.79
		}
	}

	var H, Hsamb, Hramb float64
	Hsamb = coef * math.Pow(Dx, -0.2) * math.Pow(Tair, -0.181) * math.Pow(ATdelt, 0.266) * math.Sqrt(1+1.277*es.wind)
	if Tamb != Ts {
		Hramb = es.emiss * 0.1713e-8 * (pow.E4(Tamb+459.69) - pow.E4(Ts+459.69)) / (Tamb - Ts)
	}
	H = Hsamb + Hramb
	if H < 0.0 {
		H = 1.61
	}

	es.surf = H
}

// NOR = 1 vertical pipe
// NOR = 2 horizontal PIPE

// NOR = 1 horizontal heat FLOW
// NOR = 2 heat flow down
// NOR = 3 heat flow up

type Orientation int8

const (
	FlatVerticalSurface Orientation = 1
	FlatHeatFlowDown                = 2
	FlatHeatFlowUp                  = 3
	PipeVertical                    = 1
	PipeHorizontal                  = 2
)

func Flat(o io.Writer, Tservice float64, layers []Layer, Tamb float64, es *ExternalSurface) (
	Q float64, T []float64, err error) {
	return calc(o, Tservice, layers, Tamb, es, -1.0)
}

func Cylinder(o io.Writer, Tservice float64, layers []Layer, Tamb float64, es *ExternalSurface, ODpipe float64) (
	Q float64, T []float64, err error) {
	return calc(o, Tservice, layers, Tamb, es, ODpipe)
}

func calc(o io.Writer, Tservice float64, layers []Layer, Tamb float64, es *ExternalSurface, ODpipe float64) (
	Q float64, T []float64, err error) {

	isCylinder := 0.0 < ODpipe

	// nil output
	if o == nil {
		var buf bytes.Buffer
		o = &buf
	}
	out := tabwriter.NewWriter(o, 0, 0, 1, ' ', tabwriter.AlignRight)
	defer func() {
		out.Flush()
	}()

	{
		// input data
		fmt.Fprintf(out, "HEAT FLOW AND SURFACE TEMPERATURES OF INSULATED EQUPMENT PER ASTM C-680\n")
		fmt.Fprintf(out, "\n")
		if isCylinder {
			fmt.Fprintf(out, "PIPE OUSIDE INSULLATION:\t YES\n")
			fmt.Fprintf(out, "ACTUAL PIPE DIAMETER, IN:\t %6.2f\n", ODpipe)
			fmt.Fprintf(out, "PIPE SERVICE TEMPERATURE, F:\t %6.2f\n", Tservice)
		} else {
			fmt.Fprintf(out, "EQUPMENT SERVICE TEMPERATURE, F:\t %6.2f\n", Tservice)
		}
		fmt.Fprintf(out, "AMBIENT TEMPERATURE, F:\t %6.2f\n", Tamb)
	}

	// calculate diameters per layers
	OD := make([]float64, len(layers))
	ID := make([]float64, len(layers))
	{
		ID[0] = ODpipe
		for i := range layers {
			if 0 < i {
				ID[i] = OD[i-1]
			}
			OD[i] = ID[i] + 2.0*layers[i].Thk
		}
	}

	// temperature initialization
	T = make([]float64, len(layers)+1)
	R := make([]float64, len(layers))
	K := make([]float64, len(layers))
	{
		ThkSum := 0.0
		for _, l := range layers {
			ThkSum += l.Thk
		}
		Tdelta := Tservice - Tamb
		for i := range T {
			if i == 0 {
				T[0] = Tservice
				continue
			}
			T[i] = T[i-1] - layers[i-1].Thk/ThkSum*Tdelta
		}
	}

	var iter, iterMax int64 = 0, 2000
	for ; iter < iterMax; iter++ {
		// symmary
		var Rsum float64
		if !es.isSurf {
			es.surcof(OD[len(layers)-1], T[len(layers)], Tamb, isCylinder)
		}
		Rsum = 1.0 / es.surf
		for i := range layers {
			K[i] = layers[i].Mat.ConductivityAvg(T[i], T[i+1])
			if isCylinder {
				R[i] = OD[len(layers)-1] / 2.0 * math.Log(OD[i]/ID[i]) / K[i]
			} else {
				R[i] = layers[i].Thk / K[i]
			}
			Rsum += R[i]
		}

		// heat flux
		Q = (Tservice - Tamb) / Rsum

		// iteration criteria
		tol := 0.0
		for i := range layers {
			Ts := T[i] - Q*R[i]
			tol += math.Abs(T[i+1] - Ts)
			T[i+1] = Ts // store data
		}
		if math.Abs(tol) < 1e-5 {
			break
		}
	}
	if iterMax <= iter {
		err = fmt.Errorf("not enougnt iterations")
		return
	}

	if isCylinder {
		Q = Q * math.Pi * OD[len(layers)-1] / 12.0
	}

	// TODO orientation view
	{
		// output data
		if !es.isSurf {
			fmt.Fprintf(out, "EMITTANCE:\t %6.2f\n", es.emiss)
			fmt.Fprintf(out, "WIND SPEED, MPH:\t %6.2f\n", es.wind)
		}
		fmt.Fprintf(out, "SURFACE COEF. USED, BTU/HR.SF.F:\t %6.2f\n", es.surf)
		fmt.Fprintf(out, "TOTAL HEAT FLUX, BTU/HR.SF:\t %6.2f\n", Q)
		fmt.Fprintf(out, "\n")
		fmt.Fprintf(out, "LAYER \tINSULATION \tCONDUCTIVITY \tRESISTANCE \tTEMPERATURE,F\n")
		fmt.Fprintf(out, "No \tTHICKNESS,in \tBTU.IN/HR.SF.F \tHR.SF.F/BTU \tINSIDE \tOUTSIDE\n")
		for i, l := range layers {
			fmt.Fprintf(out, "%d \t%.2f \t%.3f \t%.2f \t%.2f \t%.2f\n",
				i, l.Thk, K[i], R[i], T[i], T[i+1])
		}
	}
	return
}
