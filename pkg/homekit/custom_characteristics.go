package homekit

import (
	"github.com/brutella/hap/characteristic"
)

// Custom characteristics that don't use Temperature type to avoid C->F conversion.
// HomeKit converts any Temperature-typed characteristic to the user's unit, which
// would corrupt non-temperature readings. These types are not currently attached
// to any accessory by NewWeatherSystemModern; they are kept for future use.

// WindSpeedCharacteristic is a custom float characteristic for wind speed in mph.
type WindSpeedCharacteristic struct {
	*characteristic.Float
}

// NewWindSpeedCharacteristic returns a read/notify float characteristic for
// wind speed in mph with UUID F001-0001-1000-8000-0026BB765291, unit "mph", range 0-200, step 0.1.
func NewWindSpeedCharacteristic() *WindSpeedCharacteristic {
	c := characteristic.NewFloat("F001-0001-1000-8000-0026BB765291") // Custom UUID
	c.Format = characteristic.FormatFloat
	c.Unit = "mph"
	c.Permissions = []string{characteristic.PermissionRead, characteristic.PermissionEvents}
	c.SetMinValue(0.0)
	c.SetMaxValue(200.0)
	c.SetStepValue(0.1)
	c.SetValue(0.0)

	return &WindSpeedCharacteristic{c}
}

// WindDirectionCharacteristic is a custom float characteristic for wind direction in degrees.
type WindDirectionCharacteristic struct {
	*characteristic.Float
}

// NewWindDirectionCharacteristic returns a read/notify float characteristic for
// wind direction in degrees with UUID F002-0001-1000-8000-0026BB765291, unit "degrees", range 0-360, step 1.
func NewWindDirectionCharacteristic() *WindDirectionCharacteristic {
	c := characteristic.NewFloat("F002-0001-1000-8000-0026BB765291") // Custom UUID
	c.Format = characteristic.FormatFloat
	c.Unit = "degrees"
	c.Permissions = []string{characteristic.PermissionRead, characteristic.PermissionEvents}
	c.SetMinValue(0.0)
	c.SetMaxValue(360.0)
	c.SetStepValue(1.0)
	c.SetValue(0.0)

	return &WindDirectionCharacteristic{c}
}

// RainAccumulationCharacteristic is a custom float characteristic for rain accumulation in inches.
type RainAccumulationCharacteristic struct {
	*characteristic.Float
}

// NewRainAccumulationCharacteristic returns a read/notify float characteristic for
// rain accumulation in inches with UUID F003-0001-1000-8000-0026BB765291, unit "inches", range 0-100, step 0.001.
func NewRainAccumulationCharacteristic() *RainAccumulationCharacteristic {
	c := characteristic.NewFloat("F003-0001-1000-8000-0026BB765291") // Custom UUID
	c.Format = characteristic.FormatFloat
	c.Unit = "inches"
	c.Permissions = []string{characteristic.PermissionRead, characteristic.PermissionEvents}
	c.SetMinValue(0.0)
	c.SetMaxValue(100.0)
	c.SetStepValue(0.001)
	c.SetValue(0.0)

	return &RainAccumulationCharacteristic{c}
}

// UVIndexCharacteristic is a custom float characteristic for UV Index.
type UVIndexCharacteristic struct {
	*characteristic.Float
}

// NewUVIndexCharacteristic returns a read/notify float characteristic for
// UV Index with UUID 00000001-0000-1000-8000-0026BB765291, unit "UV Index", range 0-15, step 0.1.
func NewUVIndexCharacteristic() *UVIndexCharacteristic {
	c := characteristic.NewFloat("00000001-0000-1000-8000-0026BB765291") // HomeKit-compliant custom UUID for UV Index
	c.Format = characteristic.FormatFloat
	c.Unit = "UV Index" // Set unit to UV Index
	c.Permissions = []string{characteristic.PermissionRead, characteristic.PermissionEvents}
	c.SetMinValue(0.0)
	c.SetMaxValue(15.0)
	c.SetStepValue(0.1)
	c.SetValue(0.0)

	return &UVIndexCharacteristic{c}
}

// LightningCountCharacteristic is a custom int characteristic for lightning strike count.
type LightningCountCharacteristic struct {
	*characteristic.Int
}

// NewLightningCountCharacteristic returns a read/notify int characteristic for
// lightning strike count with UUID F005-0001-1000-8000-0026BB765291, unit "strikes", range 0-10000, step 1.
func NewLightningCountCharacteristic() *LightningCountCharacteristic {
	c := characteristic.NewInt("F005-0001-1000-8000-0026BB765291") // Custom UUID
	c.Format = characteristic.FormatInt32
	c.Unit = "strikes"
	c.Permissions = []string{characteristic.PermissionRead, characteristic.PermissionEvents}
	c.SetMinValue(0)
	c.SetMaxValue(10000)
	c.SetStepValue(1)
	_ = c.SetValue(0)

	return &LightningCountCharacteristic{c}
}

// LightningDistanceCharacteristic is a custom float characteristic for lightning distance in km.
type LightningDistanceCharacteristic struct {
	*characteristic.Float
}

// NewLightningDistanceCharacteristic returns a read/notify float characteristic for
// lightning distance in km with UUID F006-0001-1000-8000-0026BB765291, unit "km", range 0-100, step 0.1.
func NewLightningDistanceCharacteristic() *LightningDistanceCharacteristic {
	c := characteristic.NewFloat("F006-0001-1000-8000-0026BB765291") // Custom UUID
	c.Format = characteristic.FormatFloat
	c.Unit = "km"
	c.Permissions = []string{characteristic.PermissionRead, characteristic.PermissionEvents}
	c.SetMinValue(0.0)
	c.SetMaxValue(100.0)
	c.SetStepValue(0.1)
	c.SetValue(0.0)

	return &LightningDistanceCharacteristic{c}
}

// PrecipitationTypeCharacteristic is a custom int characteristic for precipitation type.
type PrecipitationTypeCharacteristic struct {
	*characteristic.Int
}

// NewPrecipitationTypeCharacteristic returns a read/notify int characteristic for
// precipitation type with UUID F007-0001-1000-8000-0026BB765291, unit "type", range 0-10, step 1.
func NewPrecipitationTypeCharacteristic() *PrecipitationTypeCharacteristic {
	c := characteristic.NewInt("F007-0001-1000-8000-0026BB765291") // Custom UUID
	c.Format = characteristic.FormatInt32
	c.Unit = "type"
	c.Permissions = []string{characteristic.PermissionRead, characteristic.PermissionEvents}
	c.SetMinValue(0)
	c.SetMaxValue(10)
	c.SetStepValue(1)
	_ = c.SetValue(0)

	return &PrecipitationTypeCharacteristic{c}
}

// PressureCharacteristic is a custom float characteristic for atmospheric pressure in mb.
type PressureCharacteristic struct {
	*characteristic.Float
}

// NewPressureCharacteristic returns a read/notify float characteristic for
// atmospheric pressure in mb with UUID F008-0001-1000-8000-0026BB765291, unit "mb", range 800-1100, step 0.1.
func NewPressureCharacteristic() *PressureCharacteristic {
	c := characteristic.NewFloat("F008-0001-1000-8000-0026BB765291") // Custom UUID
	c.Format = characteristic.FormatFloat
	c.Unit = "mb"
	c.Permissions = []string{characteristic.PermissionRead, characteristic.PermissionEvents}
	c.SetMinValue(800.0)
	c.SetMaxValue(1100.0)
	c.SetStepValue(0.1)
	c.SetValue(1013.25) // Standard atmospheric pressure

	return &PressureCharacteristic{c}
}
