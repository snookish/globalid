# globalid

`globalid` is a simple Go package for generating **globally unique, roughly time-ordered IDs**, inspired by Snowflake-style ID generation.

It is safe for concurrent use and supports custom clocks, automatic machine ID detection, and batch generation.

---

## ID Structure

Each ID is a 64-bit integer composed of:

```
[ timestamp | machine ID | sequence ]
   41 bits     10 bits     12 bits
```

* **Timestamp**: milliseconds since custom epoch (Jan 1, 2026 UTC)
* **Machine ID**: identifies the node (0–1023)
* **Sequence**: per-millisecond counter (0–4095)

---

## Usage

### Create a Generator

```go
gen, err := globalid.NewGenerator(globalid.Config{
    MachineID:   1,
    WaitForTime: true,
})
if err != nil {
    panic(err)
}
```

### Generate a Single ID

```go
id, err := gen.Generate()
if err != nil {
    panic(err)
}

fmt.Println(id.ID())      // int64
fmt.Println(id.String())  // string
```

### Generate Multiple IDs

```go
ids, err := gen.GenerateBatch(10)
if err != nil {
    panic(err)
}

for _, id := range ids {
    fmt.Println(id.ID())
}
```

---

## Auto Machine ID

You can derive the machine ID from the system MAC address:

```go
gen, err := globalid.NewGenerator(globalid.Config{
    AutoMachineID: true,
})
```

---

## Parsing an ID

```go
timestamp, machineID, sequence := id.Parse()

fmt.Println("Time:", timestamp)
fmt.Println("Machine:", machineID)
fmt.Println("Sequence:", sequence)
```

---

## Configuration Options

```go
type Config struct {
    ClockChecker  ClockChecker // Custom clock source
    MachineID     int64        // Manual machine ID (0–1023)
    AutoMachineID bool         // Use MAC address
    WaitForTime   bool         // Wait instead of error on overflow
}
```

---

## Error Handling

The generator may return:

* `ErrInvalidMachineID` – machine ID out of range
* `ErrClockBackwards` – system clock moved backwards
* `ErrSequenceExhausted` – too many IDs in one millisecond
