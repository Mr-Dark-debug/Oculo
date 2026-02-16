package analysis

import (
	"math"
	"testing"
)

func TestLinearRegression(t *testing.T) {
	// Perfect linear: y = 2x + 1
	points := []dataPoint{
		{0, 1}, {1, 3}, {2, 5}, {3, 7}, {4, 9},
	}

	slope, intercept, rSquared := linearRegression(points)

	if math.Abs(slope-2.0) > 0.001 {
		t.Errorf("expected slope=2.0, got %.3f", slope)
	}
	if math.Abs(intercept-1.0) > 0.001 {
		t.Errorf("expected intercept=1.0, got %.3f", intercept)
	}
	if math.Abs(rSquared-1.0) > 0.001 {
		t.Errorf("expected R²=1.0, got %.3f", rSquared)
	}
}

func TestLinearRegressionNoisy(t *testing.T) {
	// Noisy linear data
	points := []dataPoint{
		{0, 1.1}, {1, 2.9}, {2, 5.2}, {3, 6.8}, {4, 9.1},
	}

	slope, _, rSquared := linearRegression(points)

	// Should be approximately slope=2.0 with high R²
	if slope < 1.5 || slope > 2.5 {
		t.Errorf("expected slope ≈ 2.0, got %.3f", slope)
	}
	if rSquared < 0.95 {
		t.Errorf("expected R² > 0.95, got %.3f", rSquared)
	}
}

func TestLinearRegressionConstant(t *testing.T) {
	// All same y values — flat line
	points := []dataPoint{
		{0, 5}, {1, 5}, {2, 5}, {3, 5},
	}

	slope, intercept, rSquared := linearRegression(points)

	if math.Abs(slope) > 0.001 {
		t.Errorf("expected slope=0, got %.3f", slope)
	}
	if math.Abs(intercept-5.0) > 0.001 {
		t.Errorf("expected intercept=5.0, got %.3f", intercept)
	}
	// R² should be 1.0 for a perfect fit (even if slope=0)
	if rSquared < 0.99 {
		t.Errorf("expected R²=1.0, got %.3f", rSquared)
	}
}

func TestLinearRegressionSinglePoint(t *testing.T) {
	points := []dataPoint{{0, 5}}
	slope, _, _ := linearRegression(points)

	if slope != 0 {
		t.Errorf("expected slope=0 for single point, got %.3f", slope)
	}
}
