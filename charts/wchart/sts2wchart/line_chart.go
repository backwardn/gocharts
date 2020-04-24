package sts2wchart

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/grokify/gocharts/charts/wchart"
	"github.com/grokify/gocharts/data/statictimeseries"
	"github.com/grokify/gotilla/math/mathutil"
	"github.com/grokify/gotilla/math/ratio"
	"github.com/grokify/gotilla/strconv/strconvutil"
	"github.com/grokify/gotilla/time/month"
	"github.com/grokify/gotilla/time/timeutil"
	"github.com/wcharczuk/go-chart"
	"github.com/wcharczuk/go-chart/drawing"
)

const RatioTwoCol = ratio.RatioAcademy

// LineChartOpts is used for month and quarter interval charts.
type LineChartOpts struct {
	TitleSuffixCurrentValue     bool
	TitleSuffixCurrentValueFunc func(int64) string
	TitleSuffixCurrentDateFunc  func(time.Time) string
	Legend                      bool
	RegressionDegree            int
	NowAnnotation               bool
	MAgoAnnotation              bool
	QAgoAnnotation              bool
	YAgoAnnotation              bool
	AgoAnnotationPct            bool
	YAxisLeft                   bool
	Width                       uint64
	Height                      uint64
	AspectRatio                 float64
	Interval                    timeutil.Interval
}

func (opts *LineChartOpts) WantAnnotations() bool {
	return opts.NowAnnotation || opts.MAgoAnnotation ||
		opts.QAgoAnnotation || opts.YAgoAnnotation
}

func (opts *LineChartOpts) WantTitleSuffix() bool {
	return opts.TitleSuffixCurrentValue ||
		opts.TitleSuffixCurrentDateFunc != nil
}

func DataSeriesMonthToLineChart(ds statictimeseries.DataSeries, opts LineChartOpts) chart.Chart {
	titleParts := []string{ds.SeriesName}
	if opts.WantTitleSuffix() {
		last, err := ds.Last()
		if err == nil {
			if opts.TitleSuffixCurrentDateFunc != nil {
				str := opts.TitleSuffixCurrentDateFunc(last.Time)
				if len(str) > 0 {
					titleParts = append(titleParts, " - "+str)
				}
			}
			if opts.TitleSuffixCurrentValue {
				titleParts = append(titleParts, " - "+opts.TitleSuffixCurrentValueFunc(last.Value))
			}
		}
	}

	graph := chart.Chart{
		Title: strings.Join(titleParts, " "),
		YAxis: chart.YAxis{
			ValueFormatter: func(v interface{}) string {
				if vf, isFloat := v.(float64); isFloat {
					return strconvutil.Commify(int64(vf))
				}
				return ""
			},
		},
		Series: []chart.Series{},
	}

	if opts.YAxisLeft {
		graph.YAxis.AxisType = chart.YAxisSecondary // move Y axis to left.
	}

	if opts.Width > 0 && opts.Height > 0 {
		graph.Width = int(opts.Width)
		graph.Height = int(opts.Height)
	} else if opts.Width > 0 && opts.AspectRatio > 0 {
		graph.Width = int(opts.Width)
		graph.Height = int(ratio.WidthToHeight(float64(opts.Width), opts.AspectRatio))
	} else if opts.Height > 0 && opts.AspectRatio > 0 {
		graph.Height = int(opts.Height)
		graph.Width = int(ratio.HeightToWidth(float64(opts.Height), opts.AspectRatio))
	}

	mainSeries := chart.ContinuousSeries{}
	if opts.Interval == timeutil.Quarter {
		mainSeries = wchart.DataSeriesToContinuousSeriesQuarter(ds)
	} else {
		mainSeries = wchart.DataSeriesToContinuousSeriesMonth(ds)
	}
	mainSeries.Style = chart.Style{
		StrokeWidth: float64(3)}
	graph.Series = append(graph.Series, mainSeries)

	if opts.RegressionDegree == 1 {
		linRegSeries := &chart.LinearRegressionSeries{
			InnerSeries: mainSeries,
			Style: chart.Style{
				StrokeWidth: float64(2),
				StrokeColor: wchart.ColorOrange}}
		graph.Series = append(graph.Series, linRegSeries)
	} else if opts.RegressionDegree > 1 {
		polyRegSeries := &chart.PolynomialRegressionSeries{
			Degree:      opts.RegressionDegree,
			InnerSeries: mainSeries,
			Style: chart.Style{
				StrokeWidth: float64(2),
				StrokeColor: wchart.ColorOrange}}
		graph.Series = append(graph.Series, polyRegSeries)
	}

	if opts.Legend {
		//note we have to do this as a separate step because we need a reference to graph
		graph.Elements = []chart.Renderable{
			chart.Legend(&graph),
		}
	}

	ac := AxesCreator{
		PaddingTop: 50,
		GridMajorStyle: chart.Style{
			StrokeWidth: float64(1),
			StrokeColor: drawing.ColorFromHex("000000")},
		GridMinorStyle: chart.Style{
			StrokeWidth: float64(1),
			StrokeColor: drawing.ColorFromHex("aaaaaa")},
		XTickFormatFunc: FormatXTickTimeFunc(ds.Interval),
		YNumTicks:       7,
		YTickFormatFunc: FormatYTickFunc(ds.SeriesName)}

	minValue, maxValue := ds.MinMaxValues()
	minTime, maxTime := ds.MinMaxTimes()

	graph = ac.ChartAddAxesDataSeries(
		graph, ds.Interval, minTime, maxTime, minValue, maxValue)

	if 1 == 0 {
		tickValues := mathutil.PrettyTicks(10.0, minValue, maxValue)
		graph.YAxis.Ticks = wchart.Ticks(tickValues, strconvutil.Int64Abbreviation)

		style := chart.Style{
			StrokeWidth: float64(1),
			StrokeColor: drawing.ColorFromHex("aaaaaa")}
		styleMajor := chart.Style{
			StrokeWidth: float64(1),
			StrokeColor: drawing.ColorFromHex("000000")}

		graph.YAxis.GridLines = wchart.GridLines(tickValues, style)
		graph.YAxis.GridMajorStyle = style

		minTime, maxTime := ds.MinMaxTimes()
		xTicks, xGridlines := wchart.TicksAndGridlinesTime(
			ds.Interval, minTime, maxTime,
			styleMajor, style, FormatXTickTimeFunc(ds.Interval), true)
		graph.XAxis.Ticks = xTicks
		graph.XAxis.GridLines = xGridlines
		graph.XAxis.GridMajorStyle = style
	}

	if opts.Interval == timeutil.Month {
		annoSeries, err := dataSeriesMonthToAnnotations(ds, opts)
		if err == nil && len(annoSeries.Annotations) > 0 {
			graph.Series = append(graph.Series, annoSeries)
		}
	}
	return graph
}

func dataSeriesMonthToAnnotations(ds statictimeseries.DataSeries, opts LineChartOpts) (chart.AnnotationSeries, error) {
	annoSeries := chart.AnnotationSeries{
		Annotations: []chart.Value2{},
		Style: chart.Style{
			StrokeWidth: float64(2),
			StrokeColor: wchart.MustGetSVGColor("limegreen")},
	}

	if !opts.WantAnnotations() {
		return annoSeries, nil
	}

	xox, err := statictimeseries.NewXoXDataSeries(ds)
	if err != nil {
		return annoSeries, err
	}
	xoxLast := xox.Last()

	if opts.NowAnnotation {
		annoSeries.Annotations = append(annoSeries.Annotations, chart.Value2{
			XValue: float64(month.TimeToMonthContinuous(xoxLast.Time)),
			YValue: float64(xoxLast.Value),
			Label:  strconvutil.Int64Abbreviation(xoxLast.Value)})
	}
	if opts.MAgoAnnotation {
		annoSeries.Annotations = append(annoSeries.Annotations, chart.Value2{
			XValue: float64(month.TimeToMonthContinuous(xoxLast.TimeMonthAgo)),
			YValue: float64(xoxLast.MMAgoValue),
			Label:  "M: " + strconvutil.Int64Abbreviation(xoxLast.MMAgoValue)})
	}
	if opts.QAgoAnnotation {
		suffix := ""
		if opts.AgoAnnotationPct {
			suffix = fmt.Sprintf(", %d%%", int(xoxLast.QoQ))
		}
		annoSeries.Annotations = append(annoSeries.Annotations, chart.Value2{
			XValue: float64(month.TimeToMonthContinuous(xoxLast.TimeQuarterAgo)),
			YValue: float64(xoxLast.MQAgoValue),
			Label:  "Q: " + strconvutil.Int64Abbreviation(xoxLast.MQAgoValue) + suffix})
	}
	if opts.YAgoAnnotation {
		suffix := ""
		if opts.AgoAnnotationPct {
			suffix = fmt.Sprintf(", %d%%", int(xoxLast.YoY))
		}
		annoSeries.Annotations = append(annoSeries.Annotations, chart.Value2{
			XValue: float64(month.TimeToMonthContinuous(xoxLast.TimeYearAgo)),
			YValue: float64(xoxLast.MYAgoValue),
			Label:  "Y: " + strconvutil.Int64Abbreviation(xoxLast.MYAgoValue) + suffix})
	}
	return annoSeries, nil
}

func dataSeriesQuarterToAnnotations(ds statictimeseries.DataSeries, opts LineChartOpts) (chart.AnnotationSeries, error) {
	annoSeries := chart.AnnotationSeries{
		Annotations: []chart.Value2{},
		Style: chart.Style{
			StrokeWidth: float64(2),
			StrokeColor: wchart.MustGetSVGColor("limegreen")},
	}

	if !opts.WantAnnotations() {
		return annoSeries, nil
	}

	xox, err := statictimeseries.NewXoXDataSeries(ds)
	if err != nil {
		return annoSeries, err
	}
	xoxLast := xox.Last()

	if opts.NowAnnotation {
		annoSeries.Annotations = append(annoSeries.Annotations, chart.Value2{
			XValue: float64(month.TimeToMonthContinuous(xoxLast.Time)),
			YValue: float64(xoxLast.Value),
			Label:  strconvutil.Int64Abbreviation(xoxLast.Value)})
	}
	if opts.MAgoAnnotation {
		annoSeries.Annotations = append(annoSeries.Annotations, chart.Value2{
			XValue: float64(month.TimeToMonthContinuous(xoxLast.TimeMonthAgo)),
			YValue: float64(xoxLast.MMAgoValue),
			Label:  "M: " + strconvutil.Int64Abbreviation(xoxLast.MMAgoValue)})
	}
	if opts.QAgoAnnotation {
		suffix := ""
		if opts.AgoAnnotationPct {
			suffix = fmt.Sprintf(", %d%%", int(xoxLast.QoQ))
		}
		annoSeries.Annotations = append(annoSeries.Annotations, chart.Value2{
			XValue: float64(month.TimeToMonthContinuous(xoxLast.TimeQuarterAgo)),
			YValue: float64(xoxLast.MQAgoValue),
			Label:  "Q: " + strconvutil.Int64Abbreviation(xoxLast.MQAgoValue) + suffix})
	}
	if opts.YAgoAnnotation {
		suffix := ""
		if opts.AgoAnnotationPct {
			suffix = fmt.Sprintf(", %d%%", int(xoxLast.YoY))
		}
		annoSeries.Annotations = append(annoSeries.Annotations, chart.Value2{
			XValue: float64(month.TimeToMonthContinuous(xoxLast.TimeYearAgo)),
			YValue: float64(xoxLast.MYAgoValue),
			Label:  "Y: " + strconvutil.Int64Abbreviation(xoxLast.MYAgoValue) + suffix})
	}
	return annoSeries, nil
}

func FormatXTickTimeFunc(interval timeutil.Interval) func(time.Time) string {
	if interval == timeutil.Month {
		return func(dt time.Time) string {
			return dt.Format("Jan '06")
		}
	} else if interval == timeutil.Quarter {
		return func(dt time.Time) string {
			return timeutil.FormatQuarterYYYYQ(dt)
		}
	}
	return func(dt time.Time) string {
		return dt.Format("1/06")
	}
}

var rxMrr = regexp.MustCompile(`(?i)\bmrr\b`)

func FormatYTickFunc(seriesName string) func(int64) string {
	return func(val int64) string {
		abbr := strconvutil.Int64Abbreviation(val)
		if rxMrr.MatchString(seriesName) {
			return "$" + abbr
		}
		return abbr
	}
}

/*

YTickFormatFunc: strconvutil.Int64Abbreviation}

*/

type AxesCreator struct {
	GridMajorStyle  chart.Style
	GridMinorStyle  chart.Style
	PaddingTop      int
	YNumTicks       int
	XTickFormatFunc func(time.Time) string
	YTickFormatFunc func(int64) string
}

func (ac *AxesCreator) ChartAddAxesDataSeries(graph chart.Chart, interval timeutil.Interval, minTime, maxTime time.Time, minValue, maxValue int64) chart.Chart {
	graph.Background = chart.Style{
		Padding: chart.Box{}}
	if ac.PaddingTop > 0 {
		graph.Background.Padding.Top = ac.PaddingTop
	}

	tickValues := mathutil.PrettyTicks(ac.YNumTicks, minValue, maxValue)
	graph.YAxis.Ticks = wchart.Ticks(tickValues, ac.YTickFormatFunc)
	graph.YAxis.GridLines = wchart.GridLines(tickValues, ac.GridMinorStyle)
	graph.YAxis.GridMajorStyle = ac.GridMinorStyle

	xTicks, xGridlines := wchart.TicksAndGridlinesTime(
		interval, minTime, maxTime,
		ac.GridMajorStyle, ac.GridMinorStyle, ac.XTickFormatFunc, true)
	graph.XAxis.Ticks = xTicks
	graph.XAxis.GridLines = xGridlines
	graph.XAxis.GridMajorStyle = ac.GridMajorStyle

	return graph
}
