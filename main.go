package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/brutella/hap"
	"github.com/brutella/hap/accessory"
	"github.com/brutella/hap/characteristic"
	"github.com/brutella/hap/service"
	"github.com/pvainio/scd30"
	"periph.io/x/conn/v3/i2c/i2creg"
	"periph.io/x/host/v3"
)

var (
	db       string
	i2c      string
	interval int

	s        *accessory.Bridge
	temp     *service.TemperatureSensor
	hum      *service.HumiditySensor
	co2      *service.CarbonDioxideSensor
	co2Level *characteristic.CarbonDioxideLevel
)

func init() {
	flag.StringVar(&db, "db", "./db", "")
	flag.StringVar(&i2c, "i2c", "", "I²C bus to use")
	flag.IntVar(&interval, "interval", 5, "The time in seconds between CO₂ readings")
	flag.Parse()
}

func main() {
	// Setup the HomeKit accessory
	s = accessory.NewBridge(accessory.Info{
		Name:         "SCD30 Sensor",
		SerialNumber: "1-101625-10",
		Manufacturer: "Sensirion",
		Model:        "SCD30",
		Firmware:     "1.0",
	})

	// Ass the Temp service
	temp = service.NewTemperatureSensor()
	s.AddS(temp.S)

	// Add the Hum service
	hum = service.NewHumiditySensor()
	s.AddS(hum.S)

	// Add the CO₂ service
	co2 = service.NewCarbonDioxideSensor()
	co2Level = characteristic.NewCarbonDioxideLevel()
	co2.AddC(co2Level.C)
	s.AddS(co2.S)

	// Create the HAP server.
	server, err := hap.NewServer(hap.NewFsStore(db), s.A)
	if err != nil {
		// stop if an error happens
		log.Panic(err)
	}

	// Setup a listener for interrupts and SIGTERM signals to stop the server.
	c := make(chan os.Signal)
	signal.Notify(c, os.Interrupt)
	signal.Notify(c, syscall.SIGTERM)

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		<-c
		// Stop delivering signals.
		signal.Stop(c)
		// Cancel the context to stop the server.
		cancel()
	}()

	// Setup the SCD30 Sensor
	if _, err := host.Init(); err != nil {
		log.Fatal(err)
	}
	bus, err := i2creg.Open(i2c)
	if err != nil {
		log.Fatal(err)
	}
	defer bus.Close()
	sensor, err := scd30.Open(bus)
	if err != nil {
		log.Fatal(err)
	}
	sensor.StartMeasurements(uint16(interval))

	// Read the SCD30 Sensor
	go func() {
		for {
			time.Sleep(time.Second * time.Duration(interval))

			hasMeasurement, err := sensor.HasMeasurement()
			if err != nil {
				log.Fatalf("error %v", err)
			}
			if hasMeasurement {
				m, err := sensor.GetMeasurement()
				if err != nil {
					log.Fatalf("error %v", err)
				}

				// Temp
				temp.CurrentTemperature.SetValue(float64(m.Temperature))

				// Hum
				hum.CurrentRelativeHumidity.SetValue(float64(m.Humidity))

				// CO₂
				if m.CO2 > 1500 {
					co2.CarbonDioxideDetected.SetValue(1)
				} else {
					co2.CarbonDioxideDetected.SetValue(0)
				}
				co2Level.SetValue(float64(m.CO2))

				// Console Log
				log.Printf("Temp: %.4g°C, Hum: %.3g%%, CO₂: %.4g ppm", m.Temperature, m.Humidity, m.CO2)
			} else {
				log.Print("Failed to get a measurement...")
			}
		}
	}()

	// Run the server.
	server.ListenAndServe(ctx)
}
