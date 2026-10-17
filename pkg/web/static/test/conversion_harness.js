/**
 * @module conversion_harness
 * @description Small test harness for unit conversion helpers used by chart.html.
 * Run with: node pkg/web/static/test/conversion_harness.js
 *
 * The helpers below are local copies of the formulas, not imports, so this harness
 * checks the constants rather than chart.html itself. Exits with status 2 if any
 * case fails. Not picked up by Jest (it is not under __tests__/).
 */

/**
 * Celsius to Fahrenheit.
 * @param {number} c
 * @returns {number}
 */
function toF(c){ return (c*9.0/5.0)+32.0 }
/**
 * Fahrenheit to Celsius.
 * @param {number} f
 * @returns {number}
 */
function toC(f){ return (f-32.0)*5.0/9.0 }
/**
 * Inches of mercury to millibars (hPa).
 * @param {number} v
 * @returns {number}
 */
function inHgToMb(v){ return v*33.8638866667 }
/**
 * Millibars (hPa) to inches of mercury.
 * @param {number} v
 * @returns {number}
 */
function mbToInHg(v){ return v/33.8638866667 }
/**
 * Miles per hour to kilometres per hour.
 * @param {number} v
 * @returns {number}
 */
function mphToKmh(v){ return v*1.609344 }
/**
 * Kilometres per hour to miles per hour.
 * @param {number} v
 * @returns {number}
 */
function kmhToMph(v){ return v/1.609344 }
/**
 * Inches to millimetres.
 * @param {number} v
 * @returns {number}
 */
function inchesToMm(v){ return v*25.4 }
/**
 * Millimetres to inches.
 * @param {number} v
 * @returns {number}
 */
function mmToInches(v){ return v/25.4 }

/**
 * Absolute-tolerance float comparison.
 * @param {number} a
 * @param {number} b
 * @param {number} [tol=1e-6]
 * @returns {boolean} true when |a - b| <= tol.
 */
function approxEqual(a,b, tol=1e-6){ return Math.abs(a-b) <= tol }

/**
 * Runs every conversion case, printing ok/FAIL per case. Pressure cases use a 0.05
 * tolerance (29.92 inHg is a rounded figure for 1013.25 mb); the rest use 1e-3.
 * @returns {void} Calls process.exit(2) if any case fails.
 */
function runTests(){
  const tests = []
  // temperature
  tests.push({name: 'C->F 0', got: toF(0), want:32})
  tests.push({name: 'C->F 100', got: toF(100), want:212})
  tests.push({name: 'F->C 32', got: toC(32), want:0})
  tests.push({name: 'F->C 212', got: toC(212), want:100})
  // pressure
  tests.push({name: 'inHg->mb 29.92', got: inHgToMb(29.92), want:1013.25})
  tests.push({name: 'mb->inHg 1013.25', got: mbToInHg(1013.25), want:29.92})
  // wind
  tests.push({name: 'mph->kmh 10', got: mphToKmh(10), want:16.09344})
  tests.push({name: 'kmh->mph 16.09344', got: kmhToMph(16.09344), want:10})
  // rain
  tests.push({name: 'in->mm 1', got: inchesToMm(1), want:25.4})
  tests.push({name: 'mm->in 25.4', got: mmToInches(25.4), want:1})

  let failed = 0
  for(const t of tests){
    // pressure conversions can have slightly larger acceptable error due to constants
    const tol = (t.name.indexOf('inHg')>=0 || t.name.indexOf('mb->inHg')>=0) ? 0.05 : 1e-3
    if(!approxEqual(t.got, t.want, tol)){
      console.error('FAIL', t.name, 'got=', t.got, 'want=', t.want)
      failed++
    } else {
      console.log('ok ', t.name)
    }
  }
  if(failed>0){
    console.error('Some tests failed:', failed)
    process.exit(2)
  } else {
    console.log('All conversion tests passed')
  }
}

// Run only when executed directly, not when required.
if(require.main === module){ runTests() }
