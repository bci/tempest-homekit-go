/**
 * Browser dashboard script for the Tempest HomeKit web console.
 *
 * Loaded as a classic (non-module) script by the main dashboard page and by the
 * single-chart popout page (`/chart/<type>`). It polls the Go server's JSON API
 * (`/api/weather`, `/api/status`, `/api/alarm-status`, `/api/history`,
 * `/api/units`), renders the sensor cards, forecast, station/HomeKit status and
 * alarm list, and drives the Chart.js time-series charts. Everything lives in the
 * global scope because the HTML calls several functions from inline `onclick`
 * handlers. The jest tests under `__tests__/` load this file into jsdom and use the
 * CommonJS export of {@link updateAlarmStatus} at the bottom of the file.
 *
 * Unit conventions: the server sends temperature in °C, pressure in mb, rain
 * amounts in mm and rain rate in mm/hr; values are converted for display
 * according to {@link units}. Chart data points are stored already converted to
 * the current display units.
 *
 * @module pkg/web/static/script
 * @file Dashboard and popout-chart logic for the Tempest HomeKit web console.
 */
/**
 * A time-series point as stored in Chart.js datasets.
 * @typedef {Object} ChartPoint
 * @property {(Date|number|string)} x Timestamp (normalised to a Date by {@link validateAndSortChartData}).
 * @property {number} y Value in the current display unit.
 */
/**
 * Display unit selection.
 * @typedef {Object} Units
 * @property {string} temperature `'celsius'` or `'fahrenheit'`.
 * @property {string} wind `'mph'`, `'kph'` (set by {@link toggleUnit}) or `'kmh'` (set by {@link loadUnitsConfig}).
 * @property {string} rain `'inches'` or `'mm'`.
 * @property {string} pressure `'mb'` or `'inHg'`.
 */
/**
 * The `/api/weather` payload fields this script reads.
 * @typedef {Object} WeatherData
 * @property {number} temperature Air temperature, °C.
 * @property {number} humidity Relative humidity, %.
 * @property {number} windSpeed Average wind speed.
 * @property {number} windGust Wind gust speed.
 * @property {number} windDirection Wind direction, degrees.
 * @property {number} rainAccum Incremental rain since the previous observation, mm.
 * @property {number} rainDailyTotal Rain since local midnight, mm.
 * @property {number} rainRate Rain intensity, mm/hr.
 * @property {number} [precipitationType] 0 none, 1 rain, 2 hail, 3 rain + hail.
 * @property {number} [lightningStrikeCount] Lightning strike count.
 * @property {number} [lightningStrikeAvg] Average strike distance, km.
 * @property {number} pressure Station pressure, mb.
 * @property {number} [seaLevelPressure] Sea-level pressure, mb.
 * @property {string} [pressure_condition] Server-side pressure analysis text.
 * @property {string} [pressure_trend] Server-side pressure trend text.
 * @property {string} [weather_forecast] Server-side pressure-based forecast text.
 * @property {number} illuminance Illuminance, lux.
 * @property {number} uv UV index.
 * @property {string} lastUpdate Observation time (parseable by `Date`).
 * @property {number} [battery] Battery voltage, V (0 when unknown).
 * @property {number} [observationCount] Observations held by the server.
 * @property {number} [maxHistorySize] Server history capacity.
 * @property {Object} [unitHints] Optional hint of the units the raw data uses.
 */
/**
 * The `/api/alarm-status` payload.
 * @typedef {Object} AlarmStatus
 * @property {boolean} enabled Whether alarm processing is active.
 * @property {boolean} [disabled] Whether alarms are explicitly disabled.
 * @property {string} configPath Alarm configuration location.
 * @property {string} lastReadTime When the configuration was last read.
 * @property {number} totalAlarms Number of configured alarms.
 * @property {number} enabledAlarms Number of enabled alarms.
 * @property {Array<Object>} alarms Alarms with `name`, `enabled`, `condition`, `tags`,
 *   `channels`, `lastTriggered`, `triggeredCount`, `cooldown`, `inCooldown`,
 *   `cooldownRemaining`, `hasSchedule` and `scheduleActive`.
 */
/**
 * Whether verbose debug mode is on.
 *
 * Set from `?loglevel=debug` or `localStorage.loglevel === 'debug'` at load time,
 * and again by the DOMContentLoaded handler. It also gates the definition of the
 * `window.testCharts` / `window.diagnoseCharts` helpers.
 * @type {boolean}
 */
let DEBUG_MODE = false;
/**
 * Numeric log levels passed as the first argument to {@link debugLog}.
 * @type {{DEBUG: number, INFO: number, WARN: number, ERROR: number}}
 */
const logLevels = {
    DEBUG: 0,
    INFO: 1, 
    WARN: 2,
    ERROR: 3
};

/**
 * Query-string parameters of the current page, used to check `loglevel` (from the
 * URL parameter or localStorage) at load time.
 * @type {URLSearchParams}
 */
const urlParams = new URLSearchParams(window.location.search);
if (urlParams.get('loglevel') === 'debug' || localStorage.getItem('loglevel') === 'debug') {
    DEBUG_MODE = true;
    console.log('🐛 DEBUG MODE ENABLED - Will log calculated values, API calls and responses');
}

/**
 * Enhanced debug logger.
 *
 * A no-op under jest (when `global.__JEST__` is set) and `console.log` otherwise.
 * Callers pass a {@link logLevels} value first; the level is printed as a number
 * and is not used to filter output, and {@link DEBUG_MODE} is not consulted.
 * @type {function(...*): void}
 */
const debugLog = (typeof global !== 'undefined' && global.__JEST__) ? (() => {}) : console.log;

/**
 * Global variable that tracks the data source type reported by `/api/status`
 * (`status.dataSource.type`, e.g. `'udp'`, `'api'`, `'generated'`), used for better
 * error messaging when a weather fetch fails.
 * @type {?string}
 */
let currentDataSourceType = null;

/**
 * Current display units, initialised from localStorage and later overwritten by
 * {@link loadUnitsConfig}. Changed by {@link toggleUnit}.
 * @type {Units}
 */
let units = {
    temperature: localStorage.getItem('temperature-unit') || 'celsius',
    wind: localStorage.getItem('wind-unit') || 'mph',
    rain: localStorage.getItem('rain-unit') || 'inches',
    pressure: localStorage.getItem('pressure-unit') || 'mb'
};

/**
 * Friendly unit label helper; keeps formatting consistent with chart.html.
 *
 * @param {string} key Unit family, e.g. `'temperature'`, `'wind'`, `'rain'`, `'pressure'`.
 * @param {string} val Unit token, e.g. `'celsius'`, `'kmh'`, `'inHg'`.
 * @returns {string} The display label (e.g. `'°C'`, `'km/h'`), or `val` itself when unmapped,
 *   or `''` when `val` is empty.
 */
function prettyUnitLabel(key, val) {
    const map = {
        temperature: { celsius: '°C', fahrenheit: '°F' },
        temp: { celsius: '°C', fahrenheit: '°F' },
        wind: { mph: 'mph', kmh: 'km/h', kph: 'km/h', mps: 'm/s' },
        rain: { inches: 'in', mm: 'mm' },
        pressure: { inHg: 'inHg', mb: 'mb', hpa: 'hPa' },
        uv: { uv: 'UV' },
        illuminance: { lux: 'lux' }
    };
    const km = map[key] || map[key.toLowerCase()] || {};
    return (km[val] || val || '').toString();
}

/**
 * Load units configuration from the server (`GET /api/units`).
 *
 * Maps the server's `units` setting (`imperial`, `metric`, `sae`) onto the
 * client {@link units} object, copies `unitsPressure` verbatim, and persists all
 * four choices to localStorage.
 *
 * @async
 * @returns {Promise<boolean>} `true` on success, `false` if the request or JSON parse failed
 *   (the existing units are kept).
 */
async function loadUnitsConfig() {
    try {
        const response = await fetch('/api/units');
        const serverUnits = await response.json();
        
        // Map server units to client units
        if (serverUnits.units === 'imperial') {
            units.temperature = 'fahrenheit';
            units.wind = 'mph';
            units.rain = 'inches';
        } else if (serverUnits.units === 'metric') {
            units.temperature = 'celsius';
            units.wind = 'kmh';
            units.rain = 'mm';
        } else if (serverUnits.units === 'sae') {
            units.temperature = 'fahrenheit';
            units.wind = 'mph';
            units.rain = 'inches';
        }
        
        // Set pressure units
        units.pressure = serverUnits.unitsPressure;
        
        debugLog(logLevels.DEBUG, 'Loaded units config from server', serverUnits);
        debugLog(logLevels.DEBUG, 'Mapped client units', units);
        
        // Update localStorage for persistence
        localStorage.setItem('temperature-unit', units.temperature);
        localStorage.setItem('wind-unit', units.wind);
        localStorage.setItem('rain-unit', units.rain);
        localStorage.setItem('pressure-unit', units.pressure);
        
        return true;
    } catch (error) {
        debugLog(logLevels.ERROR, 'Failed to load units config from server, using defaults', error);
        return false;
    }
}

/**
 * Latest `/api/weather` payload, or `null` before the first successful fetch.
 * @type {?WeatherData}
 */
let weatherData = null;
/**
 * Current forecast data, stored so the forecast can be re-rendered on unit changes.
 * @type {?Object}
 */
let forecastData = null; // Store current forecast data for unit conversions
/**
 * Current `/api/status` payload, stored for unit conversions (elevation) and chart history filtering.
 * @type {?Object}
 */
let statusData = null; // Store current status data for unit conversions
/**
 * Live Chart.js instances keyed by chart name (`temperature`, `humidity`, `wind`, `rain`,
 * `pressure`, `light`, `uv`). On the popout page it instead holds `popout` (the chart)
 * and `popoutType` (the chart type string).
 * @type {Object<string, *>}
 */
const charts = {};

/**
 * Global `openChartPopout` so click handlers can call it even if
 * {@link forceChartColors} or other initialization hasn't finished.
 *
 * This mirrors the per-dataset metadata encoding used by the internal helper of
 * the same name inside {@link forceChartColors}, but omits the `incomingUnits` and
 * `theme` fields. Only defined if `window.openChartPopout` is not already set.
 * Opens `/chart/<type>?config=<json>` in a new tab; errors are logged, not thrown.
 *
 * @param {string} type Chart key in {@link charts} (also the popout URL path segment).
 * @param {string} field Observation field name passed through in the config.
 * @param {string} title Title passed through in the config.
 * @param {string} color Main line colour passed through in the config.
 * @returns {void}
 */
window.openChartPopout = window.openChartPopout || function(type, field, title, color) {
    try {
        const chartObj = charts[type];
        const datasetsMeta = [];
        if (chartObj && chartObj.data && Array.isArray(chartObj.data.datasets)) {
            chartObj.data.datasets.forEach(ds => {
                const meta = {};
                if (ds.label) meta.label = ds.label;
                if (ds.borderColor) meta.borderColor = ds.borderColor;
                if (ds.backgroundColor) meta.backgroundColor = ds.backgroundColor;
                if (ds.borderDash) meta.borderDash = ds.borderDash;
                if (ds.borderWidth !== undefined) meta.borderWidth = ds.borderWidth;
                if (ds.fill !== undefined) meta.fill = ds.fill;
                if (ds.pointRadius !== undefined) meta.pointRadius = ds.pointRadius;
                if (ds.tension !== undefined) meta.tension = ds.tension;
                if (ds.pointStyle !== undefined) meta.pointStyle = ds.pointStyle;
                if (ds.showLine !== undefined) meta.showLine = ds.showLine;
                if (ds.stepped !== undefined) meta.stepped = ds.stepped;
                if (ds.order !== undefined) meta.order = ds.order;
                if (ds.spanGaps !== undefined) meta.spanGaps = ds.spanGaps;
                if (String(ds.label).toLowerCase().includes('average')) meta.role = 'average';
                if (String(ds.label).toLowerCase().includes('trend')) meta.role = 'trend';
                if (String(ds.label).toLowerCase().includes('today') || String(ds.label).toLowerCase().includes('total')) meta.role = 'total';
                datasetsMeta.push(meta);
            });
        }

        const cfg = { type: type, field: field, title: title, color: color, units: units, datasets: datasetsMeta };
        const encoded = encodeURIComponent(JSON.stringify(cfg));
        const url = '/chart/' + type + '?config=' + encoded;
        window.open(url, '_blank');
    } catch (e) {
        debugLog(logLevels.ERROR, 'Global openChartPopout failed', e);
    }
};

/**
 * Debug hook for automated tests / headless browsers. Does not alter
 * application behavior; it simply returns current in-memory state.
 * @returns {?WeatherData} The current {@link weatherData}.
 */
window.getWeatherData = function() { return weatherData; };
/**
 * Debug hook for automated tests / headless browsers; returns the in-memory chart map.
 * @returns {Object<string, *>} The {@link charts} object.
 */
window.getCharts = function() { return charts; };
/**
 * Maximum points kept in each live chart dataset before the oldest is dropped
 * (as specified in requirements).
 * @type {number}
 */
const maxDataPoints = 1000; // As specified in requirements

/**
 * Guard counter to avoid infinite retry loops when initializing charts fails repeatedly
 * (the resilient initializer gives up after 8 attempts).
 * @type {number}
 */
let __chartInitAttempts = 0;
/**
 * Flag to indicate charts have been successfully initialized.
 * @type {boolean}
 */
let __chartsInitialized = false;
/**
 * Flag to indicate the vendored Chart.js is currently being loaded dynamically.
 * @type {boolean}
 */
let __chartVendorLoading = false;
/**
 * Whether we've attempted a global {@link destroyAllCharts} sweep already.
 * @type {boolean}
 */
let __didGlobalChartDestroy = false;
/**
 * Flag to indicate the initial weather fetch has completed (readiness gating).
 * @type {boolean}
 */
let __weatherFetched = false;
/**
 * Flag to indicate the initial status fetch has completed (readiness gating).
 * @type {boolean}
 */
let __statusFetched = false;
// Public readiness flag for tests to wait on
window.__dashboardReady = window.__dashboardReady || false;

/**
 * Set `window.__dashboardReady` once the dashboard is usable, so tests can wait on it.
 *
 * Requires charts initialized, the first weather and status fetches done, and the
 * temperature chart holding at least 2 points. If inspecting the chart throws,
 * falls back to the first three conditions only. Never throws.
 * @returns {void}
 */
function trySetDashboardReady() {
    try {
            if (window.__dashboardReady) return;
            try {
                // require charts initialized, first fetches done, and temperature chart has >=2 data points
                const tempHasData = charts && charts.temperature && charts.temperature.data && charts.temperature.data.datasets && charts.temperature.data.datasets[0] && charts.temperature.data.datasets[0].data && charts.temperature.data.datasets[0].data.length >= 2;

                if (__chartsInitialized && __weatherFetched && __statusFetched && tempHasData) {
                    window.__dashboardReady = true;
                    debugLog(logLevels.INFO, 'Dashboard ready - window.__dashboardReady set to true (includes temperature dataset length check)');
                }
            } catch (e) {
                // safe fallback: don't block if charts object is malformed
                console.warn('trySetDashboardReady: error checking dataset lengths', e);
                if (__chartsInitialized && __weatherFetched && __statusFetched) {
                    window.__dashboardReady = true;
                    debugLog(logLevels.INFO, 'Dashboard ready - window.__dashboardReady set to true (fallback)');
                }
        }
    } catch (e) {
        debugLog(logLevels.WARN, 'trySetDashboardReady encountered error', e);
    }
}

/**
 * Destroy every Chart.js instance on the page and empty {@link charts}.
 *
 * Used once as recovery when `initCharts` fails with "Canvas is already in use".
 * Walks `Chart.instances`, then `Chart.getChart()` on each known canvas id. Sets
 * {@link __didGlobalChartDestroy}. No-op when Chart.js is not loaded; never throws.
 * @returns {void}
 */
function destroyAllCharts() {
    try {
        if (typeof Chart === 'undefined') return;

        // Destroy any chart instances tracked by Chart.js itself
        try {
            // Chart.instances may be an object or Map-like; handle both
            const instances = Chart.instances ? Object.values(Chart.instances) : [];
            instances.forEach(inst => {
                try {
                    if (inst && typeof inst.destroy === 'function') {
                        inst.destroy();
                    }
                } catch (e) {
                    debugLog(logLevels.WARN, 'Failed to destroy Chart instance during global sweep', e);
                }
            });
        } catch (e) {
            debugLog(logLevels.WARN, 'Error enumerating Chart.instances', e);
        }

        // Also try Chart.getChart on known canvases as a fallback
        const canvasIds = ['temperature-chart','humidity-chart','wind-chart','rain-chart','pressure-chart','light-chart','uv-chart'];
        canvasIds.forEach(id => {
            const el = document.getElementById(id);
            if (!el) return;
            try {
                const existing = (typeof Chart.getChart === 'function') ? Chart.getChart(el) : null;
                if (existing && typeof existing.destroy === 'function') existing.destroy();
            } catch (e) {
                debugLog(logLevels.WARN, `Error destroying chart via getChart for ${id}`, e);
            }
        });

        // Clear our charts map
        Object.keys(charts).forEach(k => delete charts[k]);
        __didGlobalChartDestroy = true;
        debugLog(logLevels.INFO, 'Global Chart.js sweep completed - destroyed existing charts');
    } catch (e) {
        debugLog(logLevels.WARN, 'destroyAllCharts encountered an error', e);
    }
}
/**
 * Ensure a given dataset index exists on a chart, creating minimal `{data: []}`
 * placeholder datasets up to and including `index` if missing.
 *
 * @param {?Object} chart Chart.js instance (ignored if falsy or without `data`).
 * @param {number} index Dataset index that must exist afterwards.
 * @returns {void}
 */
function ensureDataset(chart, index) {
    if (!chart || !chart.data) return;
    if (!chart.data.datasets) chart.data.datasets = [];
    if (!chart.data.datasets[index]) {
        // Create placeholder datasets up to the requested index
        for (let i = chart.data.datasets.length; i <= index; i++) {
            chart.data.datasets[i] = { data: [] };
        }
    }
}

/**
 * Create all dashboard charts, or the single chart on the popout page.
 *
 * Destroys any Chart.js instances already bound to the known canvases first to
 * avoid "Canvas is already in use" errors on re-initialization. On the popout page
 * (a `#chart-canvas` element exists) it reads the `config` query parameter (theme,
 * units, datasets), builds one chart for the type named by the last URL path
 * segment, wires the time-range select and CSV/JSON export buttons, stores it as
 * `charts.popout` / `charts.popoutType`, and returns. Otherwise it creates the
 * seven dashboard charts (skipping any already present in {@link charts}), attaches
 * click-to-popout handlers, applies {@link forceChartColors} and adds a resize
 * handler for the rain chart axis labels.
 *
 * @returns {void}
 * @throws {ReferenceError} If Chart.js (`Chart`) is not loaded.
 * @throws {TypeError} On the dashboard page if a required chart canvas is missing.
 */
function initCharts() {
    debugLog(logLevels.DEBUG, 'Initializing all charts with configuration');
    
    // If Chart.js has already created Chart instances on these canvases,
    // destroy them first to avoid "Canvas is already in use" errors when
    // re-initializing (headless tests or dynamic reload scenarios).
    try {
        if (typeof Chart !== 'undefined' && Chart.getChart) {
            // Map canvas IDs to chart keys so we can null out the charts object
            const mapping = {
                'temperature-chart': 'temperature',
                'humidity-chart': 'humidity',
                'wind-chart': 'wind',
                'rain-chart': 'rain',
                'pressure-chart': 'pressure',
                'light-chart': 'light',
                'uv-chart': 'uv'
            };

            Object.keys(mapping).forEach(id => {
                const el = document.getElementById(id);
                if (el) {
                    try {
                        const existing = Chart.getChart(el);
                        if (existing && typeof existing.destroy === 'function') {
                            debugLog(logLevels.DEBUG, `Destroying existing Chart instance for canvas: ${id}`);
                            existing.destroy();
                        }
                    } catch (e) {
                        debugLog(logLevels.WARN, `Failed to destroy existing Chart for ${id}`, e);
                    }

                    // Ensure we remove any lingering reference in the charts map
                    const key = mapping[id];
                    if (charts[key]) {
                        try { delete charts[key]; } catch(_) { charts[key] = null; }
                    }
                }
            });
        }
    } catch (e) {
        debugLog(logLevels.WARN, 'Error while cleaning up existing Chart instances', e);
    }

    // Set Chart.js default locale to ensure 24-hour format
    Chart.defaults.font.family = '-apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif';
    
    // If this page is a chart popout, it contains a single canvas with id
    // `chart-canvas`. In that case build a minimal single chart and return.
    const popoutCanvas = document.getElementById('chart-canvas');
    if (popoutCanvas) {
        try {
            debugLog(logLevels.INFO, 'Detected chart popout canvas - creating minimal popout chart');
            
            // Read configuration from URL parameters
            const urlParams = new URLSearchParams(window.location.search);
            const configParam = urlParams.get('config');
            let popoutConfig = {};
            if (configParam) {
                try {
                    popoutConfig = JSON.parse(decodeURIComponent(configParam));
                    debugLog(logLevels.DEBUG, 'Popout config loaded from URL', popoutConfig);
                } catch (e) {
                    debugLog(logLevels.WARN, 'Failed to parse popout config from URL', e);
                }
            }
            
            // Apply theme from config if available
            if (popoutConfig.theme) {
                document.body.setAttribute('data-theme', popoutConfig.theme);
                debugLog(logLevels.INFO, 'Applied theme from popout config', { theme: popoutConfig.theme });
                
                // Update chart colors for dark themes
                if (popoutConfig.theme === 'midnight') {
                    updateChartsForDarkMode(true);
                } else {
                    updateChartsForDarkMode(false);
                }
            }
            
            // Detect chart type from URL path (/chart/temperature, /chart/humidity, etc.)
            const urlPath = window.location.pathname;
            const chartType = urlPath.split('/').pop(); // Gets 'temperature', 'humidity', etc.
            
            // Prefer units passed in the popout config (from the dashboard) so the
            // popout follows the same display units as the main console. Fall back
            // to the locally-determined `units` if no config units are provided.
            const popUnits = (popoutConfig && popoutConfig.units) ? popoutConfig.units : units;

            // Map chart types to colors and labels, using `popUnits` for unit labels
            const chartConfigs = {
                'temperature': { color: '#ff6384', label: 'Temperature', unit: popUnits.temperature === 'celsius' ? '°C' : '°F' },
                'humidity': { color: '#36a2eb', label: 'Humidity', unit: '%' },
                'wind': { color: '#ffce56', label: 'Wind Speed', unit: popUnits.wind === 'mph' ? 'mph' : 'kph' },
                'rain': { color: '#4bc0c0', label: 'Rain', unit: popUnits.rain === 'inches' ? 'in' : 'mm' },
                'pressure': { color: '#9966ff', label: 'Pressure', unit: popUnits.pressure === 'mb' ? 'mb' : 'inHg' },
                'light': { color: '#ff9f40', label: 'Light', unit: 'lux' },
                'uv': { color: '#ff6384', label: 'UV Index', unit: 'UVI' }
            };
            
            const config = chartConfigs[chartType] || { color: '#666', label: 'Data', unit: '' };
            
            const popCtx = popoutCanvas.getContext('2d');
            
            // Build datasets array - data line first (underneath), then average/trend lines on top
            // Light and UV charts don't need average lines
            const datasets = [];
            
            // Main data line is always first (drawn underneath)
            // For popout charts we want a solid data line with no point markers
            datasets.push({ 
                data: [], 
                label: config.label,
                borderColor: config.color,
                backgroundColor: config.color + '1A',
                fill: false,
                tension: 0.4,
                borderWidth: 3,
                // Hide point markers on the main data series for clarity in popouts
                pointRadius: 0,
                pointHoverRadius: 6,
                pointBackgroundColor: config.color,
                pointBorderColor: '#fff',
                pointBorderWidth: 0,
                pointHoverBackgroundColor: config.color,
                pointHoverBorderColor: '#fff',
                pointHoverBorderWidth: 2
            });
            
            if (chartType !== 'light' && chartType !== 'uv') {
                datasets.push({
                    data: [],
                    borderColor: '#00cc66',
                    backgroundColor: 'rgba(0, 204, 102, 0.2)',
                    borderDash: [5, 5],
                    borderWidth: 2,
                    fill: false,
                    pointRadius: 0,
                    tension: 0,
                    label: 'Average'
                });
            }
            
            // Pressure chart needs a trend line (on top of average)
            if (chartType === 'pressure') {
                datasets.push({
                    data: [],
                    borderColor: '#ff6384',
                    backgroundColor: 'rgba(255, 99, 132, 0.1)',
                    borderDash: [2, 2],
                    borderWidth: 1.5,
                    fill: false,
                    pointRadius: 0,
                    tension: 0,
                    label: 'Trend'
                });
            }
            
            // Rain chart needs Rain Intensity and Accumulation with dual Y-axes
            if (chartType === 'rain') {
                // Replace first dataset with Rain Intensity (rate in mm/hr)
                datasets[0] = {
                    data: [],
                    label: 'Rain Intensity',
                    borderColor: '#3b82f6',
                    backgroundColor: 'rgba(59, 130, 246, 0.2)',
                    fill: true,
                    tension: 0.4,
                    borderWidth: 2,
                    pointRadius: 0,
                    pointHoverRadius: 6,
                    yAxisID: 'y'
                };
                // Add Accumulation dataset on right Y-axis
                datasets.push({
                    data: [],
                    borderColor: '#8b5cf6',
                    backgroundColor: 'transparent',
                    borderWidth: 2,
                    fill: false,
                    pointRadius: 0,
                    tension: 0.4,
                    label: 'Accumulation',
                    yAxisID: 'y1'
                });
                // Add Today Total reference line
                datasets.push({
                    data: [],
                    borderColor: '#ff6b35',
                    backgroundColor: 'rgba(255, 107, 53, 0.1)',
                    borderDash: [3, 3],
                    borderWidth: 3,
                    fill: false,
                    pointRadius: 0,
                    tension: 0,
                    label: 'Today Total',
                    yAxisID: 'y1'
                });
            }
            
            const popChart = new Chart(popCtx, {
                type: 'line',
                data: { datasets: datasets },
                options: {
                    responsive: true,
                    maintainAspectRatio: false,
                    plugins: {
                        legend: { 
                            display: true,
                            labels: {
                                font: { size: 16, weight: 'bold' },
                                color: '#333',
                                padding: 15
                            }
                        },
                        tooltip: {
                            backgroundColor: 'rgba(20,20,20,0.95)',
                            titleColor: '#fff',
                            bodyColor: '#fff',
                            titleFont: { size: 14, weight: 'bold' },
                            bodyFont: { size: 13 },
                            padding: 12,
                            cornerRadius: 8,
                            displayColors: true,
                            callbacks: {
                                /**
                                 * Popout tooltip title: the hovered point's date and 24-hour time.
                                 * @param {Array<Object>} context Chart.js tooltip items; the first item's `parsed.x` is used.
                                 * @returns {string} E.g. `"Oct 16, 14:05"`.
                                 */
                                title: function(context) {
                                    const date = new Date(context[0].parsed.x);
                                    return date.toLocaleDateString('en-US', { month: 'short', day: '2-digit' }) + ', ' + date.toLocaleTimeString('en-GB', { hour: '2-digit', minute: '2-digit', hour12: false });
                                },
                                /**
                                 * Popout tooltip line: dataset label, value and unit. Rain values get 3 decimals and
                                 * an in/hr, mm/hr, in or mm unit depending on the dataset; others get 1 decimal.
                                 * @param {Object} context Chart.js tooltip item.
                                 * @returns {string} E.g. `"Accumulation: 0.120 in"`.
                                 */
                                label: function(context) {
                                    let value = context.parsed.y;
                                    let label = context.dataset.label || '';
                                    let formattedValue;
                                    let unit = '';
                                    
                                    // Show more precision for rain data
                                    if (chartType === 'rain') {
                                        formattedValue = value.toFixed(3);
                                        // Determine unit based on dataset
                                        if (label === 'Rain Intensity') {
                                            unit = popUnits.rain === 'inches' ? 'in/hr' : 'mm/hr';
                                        } else if (label === 'Accumulation' || label === 'Today Total') {
                                            unit = popUnits.rain === 'inches' ? 'in' : 'mm';
                                        } else {
                                            unit = config.unit;
                                        }
                                    } else {
                                        formattedValue = value.toFixed(1);
                                        unit = config.unit;
                                    }
                                    return label + ': ' + formattedValue + ' ' + unit;
                                }
                            }
                        }
                    },
                    scales: {
                        x: {
                            display: true,
                            type: 'time',
                            time: {
                                displayFormats: { minute: 'HH:mm', hour: 'HH:mm', day: 'MMM dd' },
                                tooltipFormat: 'MMM dd, HH:mm'
                            },
                            grid: { 
                                display: true, 
                                color: 'rgba(0,0,0,0.12)',
                                lineWidth: 1
                            },
                            ticks: { 
                                maxTicksLimit: 8, 
                                color: '#333', 
                                font: { size: 14, weight: '500' },
                                padding: 8,
                                /**
                                 * Popout x-axis tick label: 24-hour `HH:mm` time.
                                 * @param {number} value Tick value (epoch milliseconds).
                                 * @returns {string}
                                 */
                                callback: function(value){ 
                                    return new Date(value).toLocaleTimeString('en-GB', { hour: '2-digit', minute: '2-digit', hour12: false }); 
                                } 
                            }
                        },
                        y: {
                            display: true,
                            beginAtZero: (chartType === 'rain' || chartType === 'humidity' || chartType === 'uv' || chartType === 'light'),
                            suggestedMin: (chartType === 'rain' || chartType === 'humidity' || chartType === 'uv' || chartType === 'light') ? 0 : undefined,
                            suggestedMax: chartType === 'rain' ? 0.1 : undefined,
                            position: chartType === 'rain' ? 'left' : 'left',
                            grid: { 
                                display: true, 
                                color: chartType === 'rain' ? 'rgba(59, 130, 246, 0.1)' : 'rgba(0,0,0,0.12)',
                                lineWidth: 1
                            },
                            ticks: { 
                                maxTicksLimit: 8, 
                                color: chartType === 'rain' ? '#3b82f6' : '#333', 
                                font: { size: 14, weight: '500' },
                                padding: 8,
                                /**
                                 * Popout y-axis tick label: 3 decimals for rain charts, 1 decimal otherwise.
                                 * @param {number} value Tick value.
                                 * @returns {string}
                                 */
                                callback: function(value){ 
                                    // For rain charts, show more decimal places for small values
                                    if (chartType === 'rain') {
                                        return value.toFixed(3);
                                    }
                                    return value.toFixed(1); 
                                } 
                            },
                                title: { 
                                display: true, 
                                text: chartType === 'rain' ? (popUnits.rain === 'inches' ? 'Rain Intensity (in/hr)' : 'Rain Intensity (mm/hr)') : config.unit, 
                                color: chartType === 'rain' ? '#3b82f6' : '#333', 
                                font: { size: 16, weight: 'bold' },
                                padding: { top: 10, bottom: 10 }
                            }
                        },
                        y1: chartType === 'rain' ? {
                            type: 'linear',
                            display: true,
                            position: 'right',
                            beginAtZero: true,
                            grid: {
                                display: false
                            },
                            ticks: {
                                color: '#8b5cf6',
                                font: { size: 14, weight: '500' },
                                padding: 8,
                                /**
                                 * Popout rain accumulation (right axis) tick label with 3 decimals.
                                 * @param {number} value Tick value.
                                 * @returns {string}
                                 */
                                callback: function(value) {
                                    return value.toFixed(3);
                                }
                            },
                            title: {
                                display: true,
                                text: popUnits.rain === 'inches' ? 'Accumulation (in)' : 'Accumulation (mm)',
                                color: '#8b5cf6',
                                font: { size: 16, weight: 'bold' },
                                padding: { top: 10, bottom: 10 }
                            }
                        } : undefined
                    },
                    elements: {
                        // Default to no visible points on popout data lines; hover still highlights
                        point: { 
                            radius: 0, 
                            hoverRadius: 6, 
                            backgroundColor: config.color, 
                            borderWidth: 0,
                            borderColor: '#fff'
                        },
                        line: { 
                            borderWidth: 3, 
                            borderJoinStyle: 'round', 
                            tension: 0.4 
                        }
                    },
                    interaction: { intersect: false, mode: 'index' }
                }
            });
            
            // Store chart type for data updates
            charts.popoutType = chartType;
            charts.popout = popChart;
            
            // Update the page title to reflect the chart type
            const chartTitle = config.label + ' Chart';
            document.title = chartTitle + ' - Tempest';
            
            const h1Element = document.querySelector('#chart-root h1');
            const titleElement = document.getElementById('chart-title');
            if (h1Element) {
                h1Element.textContent = chartTitle;
            }
            if (titleElement) {
                titleElement.textContent = chartTitle;
            }
            
            // Set up time range selector
            const timeRangeSelect = document.getElementById('time-range');
            if (timeRangeSelect) {
                timeRangeSelect.addEventListener('change', function() {
                    const hours = parseInt(this.value);
                    filterChartDataByTime(charts.popout, hours);
                });
            }
            
            // Set up export buttons
            const exportCsvBtn = document.getElementById('export-csv');
            if (exportCsvBtn) {
                exportCsvBtn.addEventListener('click', function() {
                    exportChartData(charts.popout, chartType, 'csv');
                });
            }
            
            const exportJsonBtn = document.getElementById('export-json');
            if (exportJsonBtn) {
                exportJsonBtn.addEventListener('click', function() {
                    exportChartData(charts.popout, chartType, 'json');
                });
            }
            
            debugLog(logLevels.INFO, 'Popout chart created successfully', { type: chartType, config: config });
            
            // Add window resize handler for popout charts
            let resizeTimeout;
            window.addEventListener('resize', function() {
                clearTimeout(resizeTimeout);
                resizeTimeout = setTimeout(function() {
                    if (charts.popout) {
                        charts.popout.resize();
                        debugLog(logLevels.DEBUG, 'Popout chart resized to fit window');
                    }
                }, 250); // Debounce resize events
            });
        } catch (e) {
            debugLog(logLevels.ERROR, 'Failed to create popout chart', e);
        }
        return;
    }

    /**
     * Filter chart data by time range (for popout charts) by setting the x-axis
     * min/max, then stretch the two-point horizontal reference lines to the new range.
     *
     * The data itself is not removed; only the visible window changes.
     *
     * @param {Chart} chart The Chart.js instance; ignored if it has no x scale options.
     * @param {number} hours Number of hours to show, ending now (0 = all data).
     * @returns {void}
     */
    function filterChartDataByTime(chart, hours) {
        if (!chart || !chart.options || !chart.options.scales || !chart.options.scales.x) {
            return;
        }
        
        let minTime, maxTime;
        
        if (hours === 0) {
            // Show all data - remove time constraints
            delete chart.options.scales.x.min;
            delete chart.options.scales.x.max;
            
            // Get the actual data range for updating horizontal lines
            const lastDataset = chart.data.datasets[chart.data.datasets.length - 1];
            if (lastDataset && lastDataset.data && lastDataset.data.length > 0) {
                minTime = lastDataset.data[0].x;
                maxTime = lastDataset.data[lastDataset.data.length - 1].x;
            }
        } else {
            // Filter to last N hours
            const now = new Date();
            const startTime = new Date(now.getTime() - (hours * 60 * 60 * 1000));
            minTime = startTime.getTime();
            maxTime = now.getTime();
            chart.options.scales.x.min = minTime;
            chart.options.scales.x.max = maxTime;
        }
        
        // Update horizontal lines (Average, Trend, Today Total) to match the filtered time range
        // For rain chart, update the "Today Total" line (dataset 2) if it exists
        const chartType = charts.popoutType;
        if (chartType === 'rain' && chart.data.datasets[2] && chart.data.datasets[2].data.length === 2 && minTime && maxTime) {
            const currentYValue = chart.data.datasets[2].data[0].y;
            chart.data.datasets[2].data = [
                { x: minTime, y: currentYValue },
                { x: maxTime, y: currentYValue }
            ];
        }
        
        // Update Average line (dataset 0) to match filtered range - skip for light and UV
        if (chartType !== 'light' && chartType !== 'uv' && chart.data.datasets[0] && chart.data.datasets[0].data.length === 2 && minTime && maxTime) {
            const currentAvgValue = chart.data.datasets[0].data[0].y;
            chart.data.datasets[0].data = [
                { x: minTime, y: currentAvgValue },
                { x: maxTime, y: currentAvgValue }
            ];
        }
        
        // Update Trend line (dataset 1) for pressure charts if needed
        if (chartType === 'pressure' && chart.data.datasets[1] && minTime && maxTime) {
            // Trend line needs recalculation based on visible data, but for now just update endpoints
            // The main data filtering happens via x-axis min/max, so trend will be recalculated on next update
        }
        
        chart.update();
        debugLog(logLevels.INFO, 'Chart time range updated', { hours: hours, chartType: chartType });
    }

    /**
     * Export chart data in CSV or JSON format, as a browser download named
     * `tempest-<chartType>-<timestamp>.<format>`. Shows an alert when there is no data.
     * Any other `format` value does nothing.
     *
     * @param {Chart} chart The Chart.js instance.
     * @param {string} chartType Type of chart (temperature, rain, etc.), used in the filename.
     * @param {string} format Export format (`'csv'` or `'json'`).
     * @returns {void}
     */
    function exportChartData(chart, chartType, format) {
        if (!chart || !chart.data || !chart.data.datasets || chart.data.datasets.length === 0) {
            alert('No data available to export');
            return;
        }

        const timestamp = new Date().toISOString().replace(/[:.]/g, '-').substring(0, 19);
        const filename = `tempest-${chartType}-${timestamp}.${format}`;

        if (format === 'csv') {
            exportAsCSV(chart, filename);
        } else if (format === 'json') {
            exportAsJSON(chart, filename);
        }
    }

    /**
     * Export chart data as CSV: a `Timestamp` column (ISO 8601) plus one column per
     * dataset, one row per distinct x value across all datasets.
     *
     * @param {Chart} chart The Chart.js instance.
     * @param {string} filename Download file name.
     * @returns {void}
     */
    function exportAsCSV(chart, filename) {
        let csv = 'Timestamp,';
        
        // Add headers for each dataset
        chart.data.datasets.forEach((dataset, index) => {
            csv += dataset.label || `Dataset ${index}`;
            if (index < chart.data.datasets.length - 1) csv += ',';
        });
        csv += '\n';

        // Get all unique timestamps
        const timestamps = new Set();
        chart.data.datasets.forEach(dataset => {
            if (dataset.data) {
                dataset.data.forEach(point => {
                    if (point && point.x) timestamps.add(point.x);
                });
            }
        });

        // Sort timestamps
        const sortedTimestamps = Array.from(timestamps).sort((a, b) => a - b);

        // Add data rows
        sortedTimestamps.forEach(timestamp => {
            const date = new Date(timestamp);
            csv += date.toISOString() + ',';
            
            chart.data.datasets.forEach((dataset, index) => {
                const point = dataset.data.find(p => p.x === timestamp);
                csv += point ? (point.y !== undefined ? point.y : '') : '';
                if (index < chart.data.datasets.length - 1) csv += ',';
            });
            csv += '\n';
        });

        downloadFile(csv, filename, 'text/csv');
    }

    /**
     * Export chart data as pretty-printed JSON with the chart type, export time, and
     * each dataset's label, colours and `{timestamp, value}` points.
     *
     * @param {Chart} chart The Chart.js instance.
     * @param {string} filename Download file name.
     * @returns {void}
     */
    function exportAsJSON(chart, filename) {
        const exportData = {
            chartType: charts.popoutType || 'unknown',
            exportTime: new Date().toISOString(),
            datasets: chart.data.datasets.map(dataset => ({
                label: dataset.label,
                borderColor: dataset.borderColor,
                backgroundColor: dataset.backgroundColor,
                data: dataset.data.map(point => ({
                    timestamp: new Date(point.x).toISOString(),
                    value: point.y
                }))
            }))
        };

        const json = JSON.stringify(exportData, null, 2);
        downloadFile(json, filename, 'application/json');
    }

    /**
     * Trigger a file download in the browser via a temporary Blob URL and `<a download>` click.
     *
     * @param {string} content File contents.
     * @param {string} filename Suggested file name.
     * @param {string} mimeType MIME type of the Blob.
     * @returns {void}
     */
    function downloadFile(content, filename, mimeType) {
        const blob = new Blob([content], { type: mimeType });
        const url = URL.createObjectURL(blob);
        const link = document.createElement('a');
        link.href = url;
        link.download = filename;
        document.body.appendChild(link);
        link.click();
        document.body.removeChild(link);
        URL.revokeObjectURL(url);
        debugLog(logLevels.INFO, 'File downloaded', { filename: filename, size: content.length });
    }

    const ctxTemp = document.getElementById('temperature-chart').getContext('2d');
    const ctxHumidity = document.getElementById('humidity-chart').getContext('2d');
    const ctxWind = document.getElementById('wind-chart').getContext('2d');
    const ctxRain = document.getElementById('rain-chart').getContext('2d');
    const ctxPressure = document.getElementById('pressure-chart').getContext('2d');
    const ctxLight = document.getElementById('light-chart').getContext('2d');
    const uvElement = document.getElementById('uv-chart');
    const ctxUV = uvElement ? uvElement.getContext('2d') : null;

    const chartConfig = {
        type: 'line',
        options: {
            responsive: true,
            maintainAspectRatio: false,
            plugins: {
                legend: { display: false },
                tooltip: {
                    backgroundColor: 'rgba(20,20,20,0.9)',
                    titleColor: '#fff',
                    bodyColor: '#fff',
                    padding: 8,
                    cornerRadius: 6,
                    callbacks: {
                        /**
                         * Dashboard tooltip title: the hovered point's date and 24-hour time.
                         * @param {Array<Object>} context Chart.js tooltip items; the first item's `parsed.x` is used.
                         * @returns {string}
                         */
                        title: function(context) {
                            const date = new Date(context[0].parsed.x);
                            return date.toLocaleDateString('en-US', { month: 'short', day: '2-digit' }) + ', ' + date.toLocaleTimeString('en-GB', { hour: '2-digit', minute: '2-digit', hour12: false });
                        }
                    }
                }
            },
            scales: {
                x: {
                    display: true,
                    type: 'time',
                    time: {
                        displayFormats: { minute: 'HH:mm', hour: 'HH:mm', day: 'MMM dd' },
                        tooltipFormat: 'MMM dd, HH:mm'
                    },
                    grid: { display: true, color: 'rgba(0,0,0,0.06)' },
                    /**
                     * The x-axis tick `callback` on the next line formats the tick (epoch ms) as 24-hour `HH:mm`.
                     */
                    ticks: { maxTicksLimit: 6, color: '#666', font: { size: 11 }, callback: function(value){ return new Date(value).toLocaleTimeString('en-GB', { hour: '2-digit', minute: '2-digit', hour12: false }); } },
                    title: { display: false }
                },
                y: {
                    display: true,
                    grid: { display: true, color: 'rgba(0,0,0,0.06)' },
                    /**
                     * The y-axis tick `callback` on the next line formats the value with 1 decimal.
                     */
                    ticks: { maxTicksLimit: 5, color: '#444', font: { size: 12 }, callback: function(value){ return value.toFixed(1); } },
                    title: { display: true, text: 'Value', color: '#444', font: { size: 12, weight: '600' } }
                }
            },
            elements: {
                point: { radius: 3, hoverRadius: 5, backgroundColor: '#fff', borderWidth: 1 },
                line: { borderWidth: 2.5, borderJoinStyle: 'round', tension: 0.35 }
            },
            interaction: { intersect: false, mode: 'index' }
        }
    };

    // If a chart already exists in memory, do not recreate it (prevents reuse errors)
    if (charts.temperature && typeof charts.temperature.update === 'function') {
        debugLog(logLevels.INFO, 'Temperature chart already exists in memory; skipping creation');
    } else {
        charts.temperature = new Chart(ctxTemp, {
        ...chartConfig,
        data: {
            datasets: [{
                data: [],
                borderColor: '#ff6384',
                backgroundColor: 'rgba(255, 99, 132, 0.1)',
                fill: false,
                tension: 0.4,
                spanGaps: false,
                label: 'Temperature'
            }, {
                data: [],
                borderColor: '#00cc66',
                backgroundColor: 'rgba(0, 204, 102, 0.2)',
                borderDash: [5, 5],
                borderWidth: 2,
                fill: false,
                pointRadius: 0,
                tension: 0,
                label: 'Average'
            }]
        },
        options: {
            ...chartConfig.options,
            scales: {
                ...chartConfig.options.scales,
                y: {
                    ...chartConfig.options.scales.y,
                    title: {
                        display: true,
                        text: units.temperature === 'celsius' ? '°C' : '°F',
                        color: '#444',
                        font: { size: 12, weight: '600' }
                    }
                }
            }
        }
    });
        // clicking on a chart should open a pop-out detailed chart page
        document.getElementById('temperature-chart').addEventListener('click', function(){
            openChartPopout('temperature', 'temperature', 'Temperature', charts.temperature.data.datasets[0].borderColor);
        });
    }
    
    // Force the colors after creation
    charts.temperature.data.datasets[1].borderColor = '#00cc66';
    charts.temperature.data.datasets[1].backgroundColor = 'rgba(0, 204, 102, 0.2)';
    
    debugLog(logLevels.INFO, 'Temperature chart created with colors:', {
        dataColor: charts.temperature.data.datasets[0].borderColor,
        avgColor: charts.temperature.data.datasets[1].borderColor
    });

    if (charts.humidity && typeof charts.humidity.update === 'function') {
        debugLog(logLevels.INFO, 'Humidity chart already exists in memory; skipping creation');
    } else {
        charts.humidity = new Chart(ctxHumidity, {
        ...chartConfig,
        data: {
            datasets: [{
                data: [],
                borderColor: 'rgba(54, 162, 235, 0.8)',
                backgroundColor: 'rgba(54, 162, 235, 0.1)',
                fill: false,
                tension: 0.4,
                spanGaps: false,
                label: 'Humidity'
            }, {
                data: [],
                borderColor: '#ff8533',
                backgroundColor: 'rgba(255, 133, 51, 0.2)',
                borderDash: [5, 5],
                borderWidth: 2,
                fill: false,
                pointRadius: 0,
                tension: 0,
                label: 'Average'
            }]
        },
        options: {
            ...chartConfig.options,
            scales: {
                ...chartConfig.options.scales,
                y: {
                    ...chartConfig.options.scales.y,
                    title: {
                        display: true,
                        text: '%',
                        color: '#444',
                        font: { size: 12, weight: '600' }
                    }
                }
            }
        }
    });
        document.getElementById('humidity-chart').addEventListener('click', function(){
            openChartPopout('humidity', 'humidity', 'Humidity', charts.humidity.data.datasets[0].borderColor);
        });
    }
    
    // Force the colors after creation
    charts.humidity.data.datasets[1].borderColor = '#ff8533';
    charts.humidity.data.datasets[1].backgroundColor = 'rgba(255, 133, 51, 0.2)';
    
    debugLog(logLevels.INFO, 'Humidity chart created with colors:', {
        dataColor: charts.humidity.data.datasets[0].borderColor,
        avgColor: charts.humidity.data.datasets[1].borderColor
    });

    if (charts.wind && typeof charts.wind.update === 'function') {
        debugLog(logLevels.INFO, 'Wind chart already exists in memory; skipping creation');
    } else {
        charts.wind = new Chart(ctxWind, {
        ...chartConfig,
        data: {
            datasets: [{
                data: [],
                borderColor: 'rgba(75, 192, 192, 0.8)',
                backgroundColor: 'rgba(75, 192, 192, 0.1)',
                fill: false,
                tension: 0.4,
                spanGaps: false,
                label: 'Wind'
            }, {
                data: [],
                borderColor: '#ff4d4d',
                backgroundColor: 'rgba(255, 77, 77, 0.2)',
                borderDash: [5, 5],
                borderWidth: 2,
                fill: false,
                pointRadius: 0,
                tension: 0,
                label: 'Average'
            }]
        },
        options: {
            ...chartConfig.options,
            scales: {
                ...chartConfig.options.scales,
                y: {
                    ...chartConfig.options.scales.y,
                    title: {
                        display: true,
                        text: units.wind === 'mph' ? 'mph' : 'kph',
                        color: '#444',
                        font: { size: 12, weight: '600' }
                    }
                }
            }
        }
    });
        document.getElementById('wind-chart').addEventListener('click', function(){
            openChartPopout('wind', 'windSpeed', 'Wind Speed', charts.wind.data.datasets[0].borderColor);
        });
    }

    if (charts.rain && typeof charts.rain.update === 'function') {
        debugLog(logLevels.INFO, 'Rain chart already exists in memory; skipping creation');
    } else {
        charts.rain = new Chart(ctxRain, {
        ...chartConfig,
        data: {
            datasets: [{
                data: [],
                borderColor: '#3b82f6',
                backgroundColor: 'rgba(59, 130, 246, 0.3)',
                fill: true,
                tension: 0.4,
                spanGaps: false,
                label: 'Rain Intensity',
                pointRadius: 1,
                pointHoverRadius: 4,
                yAxisID: 'y'
            }, {
                data: [],
                borderColor: '#8b5cf6',
                backgroundColor: 'rgba(139, 92, 246, 0.1)',
                borderWidth: 3,
                fill: false,
                pointRadius: 0,
                tension: 0.4,
                label: 'Rain Accumulation',
                yAxisID: 'y1'
            }]
        },
        options: {
            ...chartConfig.options,
            interaction: {
                intersect: false,
                mode: 'index'
            },
            scales: {
                x: chartConfig.options.scales.x,
                y: {
                    type: 'linear',
                    display: true,
                    position: 'left',
                    beginAtZero: true,
                    title: {
                        display: window.innerWidth >= 600,
                        text: units.rain === 'inches' ? 'Rain Intensity (in/hr)' : 'Rain Intensity (mm/hr)',
                        color: '#3b82f6',
                        font: { size: 11, weight: 'bold' }
                    },
                    grid: {
                        display: true,
                        color: 'rgba(59, 130, 246, 0.1)'
                    },
                    ticks: {
                        display: window.innerWidth >= 600,
                        color: '#3b82f6',
                        font: { size: 8 },
                        padding: 2,
                        maxTicksLimit: 4,
                        /**
                         * Rain intensity (left axis) tick label with 2 decimals.
                         * @param {number} value Tick value.
                         * @returns {string}
                         */
                        callback: function(value) {
                            return value.toFixed(2);
                        }
                    }
                },
                y1: {
                    type: 'linear',
                    display: true,
                    position: 'right',
                    beginAtZero: true,
                    suggestedMax: 1.0,
                    title: {
                        display: window.innerWidth >= 600,
                        text: units.rain === 'inches' ? 'Accumulation (in)' : 'Accumulation (mm)',
                        color: '#8b5cf6',
                        font: { size: 11, weight: 'bold' }
                    },
                    grid: {
                        display: false
                    },
                    ticks: {
                        display: window.innerWidth >= 600,
                        color: '#8b5cf6',
                        font: { size: 8 },
                        padding: 2,
                        maxTicksLimit: 4,
                        /**
                         * Rain accumulation (right axis) tick label with 2 decimals.
                         * @param {number} value Tick value.
                         * @returns {string}
                         */
                        callback: function(value) {
                            return value.toFixed(2);
                        }
                    }
                }
            },
            plugins: {
                ...chartConfig.options.plugins,
                tooltip: {
                    ...chartConfig.options.plugins.tooltip,
                    callbacks: {
                        /**
                         * Rain chart tooltip line: dataset 0 is shown as intensity (in/hr or mm/hr),
                         * every other dataset as an amount (in or mm), each with 3 decimals.
                         * @param {Object} context Chart.js tooltip item.
                         * @returns {string}
                         */
                        label: function(context) {
                            let label = context.dataset.label || '';
                            if (label) {
                                label += ': ';
                            }
                            if (context.parsed.y !== null) {
                                if (context.datasetIndex === 0) {
                                    // Rain Intensity
                                    const unit = units.rain === 'inches' ? 'in/hr' : 'mm/hr';
                                    label += context.parsed.y.toFixed(3) + ' ' + unit;
                                } else {
                                    // Rain Accumulation
                                    const unit = units.rain === 'inches' ? 'in' : 'mm';
                                    label += context.parsed.y.toFixed(3) + ' ' + unit;
                                }
                            }
                            return label;
                        }
                    }
                }
            }
        }
    });
        document.getElementById('rain-chart').addEventListener('click', function(){
            openChartPopout('rain', 'rainAccum', 'Rain', charts.rain.data.datasets[0].borderColor);
        });
    }

    if (charts.pressure && typeof charts.pressure.update === 'function') {
        debugLog(logLevels.INFO, 'Pressure chart already exists in memory; skipping creation');
    } else {
        charts.pressure = new Chart(ctxPressure, {
        ...chartConfig,
        data: {
            datasets: [{
                data: [],
                borderColor: 'rgba(255, 159, 64, 0.8)',
                backgroundColor: 'rgba(255, 159, 64, 0.1)',
                fill: false,
                tension: 0.4,
                spanGaps: false,
                label: 'Pressure'
            }, {
                data: [],
                borderColor: '#4080ff',
                backgroundColor: 'rgba(64, 128, 255, 0.2)',
                borderDash: [5, 5],
                borderWidth: 2,
                fill: false,
                pointRadius: 0,
                tension: 0,
                label: 'Average'
            }, {
                data: [],
                borderColor: '#ff6384',
                backgroundColor: 'rgba(255, 99, 132, 0.1)',
                borderDash: [2, 2],
                borderWidth: 1.5,
                fill: false,
                pointRadius: 0,
                tension: 0,
                label: 'Trend'
            }]
        },
        options: {
            ...chartConfig.options,
            scales: {
                ...chartConfig.options.scales,
                y: {
                    ...chartConfig.options.scales.y,
                    title: {
                        display: true,
                        text: units.pressure === 'mb' ? 'mb' : 'inHg',
                        color: '#444',
                        font: { size: 12, weight: '600' }
                    }
                }
            }
        }
    });
        document.getElementById('pressure-chart').addEventListener('click', function(){
            openChartPopout('pressure', 'pressure', 'Pressure', charts.pressure.data.datasets[0].borderColor);
        });
    }

    if (charts.light && typeof charts.light.update === 'function') {
        debugLog(logLevels.INFO, 'Light chart already exists in memory; skipping creation');
    } else {
        charts.light = new Chart(ctxLight, {
        ...chartConfig,
        data: {
            datasets: [{
                data: [],
                borderColor: 'rgba(255, 205, 86, 0.8)',
                backgroundColor: 'rgba(255, 205, 86, 0.1)',
                fill: false,
                tension: 0.4,
                spanGaps: false,
                label: 'Light'
            }]
        },
        options: {
            ...chartConfig.options,
            scales: {
                ...chartConfig.options.scales,
                y: {
                    ...chartConfig.options.scales.y,
                    title: {
                        display: true,
                        text: 'lux',
                        color: '#444',
                        font: { size: 12, weight: '600' }
                    }
                }
            }
        }
    });
        document.getElementById('light-chart').addEventListener('click', function(){
            openChartPopout('light', 'illuminance', 'Illuminance', charts.light.data.datasets[0].borderColor);
        });
    }

    // Check if UV chart element exists before creating chart
    if (ctxUV) {
        if (charts.uv && typeof charts.uv.update === 'function') {
            debugLog(logLevels.INFO, 'UV chart already exists in memory; skipping creation');
        } else {
            charts.uv = new Chart(ctxUV, {
            ...chartConfig,
            data: {
                datasets: [{
                    data: [],
                    borderColor: 'rgba(153, 102, 255, 0.8)',
                    backgroundColor: 'rgba(153, 102, 255, 0.1)',
                    fill: false,
                    tension: 0.4,
                    spanGaps: false,
                    label: 'UV Index'
                }]
            },
            options: {
                ...chartConfig.options,
                scales: {
                    ...chartConfig.options.scales,
                    y: {
                        ...chartConfig.options.scales.y,
                        title: {
                            display: true,
                            text: 'UVI',
                            color: '#444',
                            font: { size: 12, weight: '600' }
                        }
                    }
                }
            }
        });
            debugLog(logLevels.DEBUG, 'UV chart created successfully');
        }
    } else {
        debugLog(logLevels.DEBUG, 'UV chart element not found, skipping UV chart creation');
    }
    
    // Force all chart colors after creation
    forceChartColors();
    
    // Add window resize handler to update rain chart labels responsively
    window.addEventListener('resize', function() {
        if (charts.rain && charts.rain.options && charts.rain.options.scales) {
            const width = window.innerWidth;
            const showTitles = width >= 600;
            const showTicks = width >= 600;
            
            if (charts.rain.options.scales.y && charts.rain.options.scales.y.title) {
                charts.rain.options.scales.y.title.display = showTitles;
            }
            if (charts.rain.options.scales.y && charts.rain.options.scales.y.ticks) {
                charts.rain.options.scales.y.ticks.display = showTicks;
            }
            if (charts.rain.options.scales.y1 && charts.rain.options.scales.y1.title) {
                charts.rain.options.scales.y1.title.display = showTitles;
            }
            if (charts.rain.options.scales.y1 && charts.rain.options.scales.y1.ticks) {
                charts.rain.options.scales.y1.ticks.display = showTicks;
            }
            charts.rain.update('none');
        }
    });
}

/**
 * Force the complementary colour pairs onto each dashboard chart's secondary datasets.
 *
 * Creates any missing secondary datasets via {@link ensureDataset} first:
 * temperature (red data, green average), humidity (blue, orange average), wind
 * (teal, bright red average, width 3), rain (datasets 1-3: yellow-green, orange
 * 24h total, cyan window total) and pressure (orange, blue average). Light and UV
 * have no average line. Also attaches the UV chart's click-to-popout handler when
 * the wind chart exists.
 * @returns {void}
 */
function forceChartColors() {
    debugLog(logLevels.INFO, '🎨 Forcing chart colors to complementary pairs');
    
    // Temperature: Red data → Green average
    if (charts.temperature) {
        ensureDataset(charts.temperature, 1);
        charts.temperature.data.datasets[1].borderColor = '#00cc66';
        charts.temperature.data.datasets[1].backgroundColor = 'rgba(0, 204, 102, 0.2)';
        charts.temperature.update('none');
    }
    
    // Humidity: Blue data → Orange average  
    if (charts.humidity) {
        ensureDataset(charts.humidity, 1);
        charts.humidity.data.datasets[1].borderColor = '#ff8533';
        charts.humidity.data.datasets[1].backgroundColor = 'rgba(255, 133, 51, 0.2)';
        charts.humidity.update('none');
    }
    
    // Wind: Teal data → Bright Red average (more visible)
    if (charts.wind) {
        ensureDataset(charts.wind, 1);
        charts.wind.data.datasets[1].borderColor = '#FF0000';
        charts.wind.data.datasets[1].backgroundColor = 'rgba(255, 0, 0, 0.3)';
        charts.wind.data.datasets[1].borderWidth = 3;
        charts.wind.update('none');
        
        debugLog(logLevels.INFO, '🌬️ Wind chart colors applied:', {
            dataColor: charts.wind.data.datasets[0].borderColor,
            avgColor: charts.wind.data.datasets[1].borderColor,
            avgDataPoints: charts.wind.data.datasets[1].data.length
        });
        // Attach UV chart click handler if the uv canvas exists
        const uvCanvasEl = document.getElementById('uv-chart');
        if (uvCanvasEl) {
            uvCanvasEl.addEventListener('click', function(){
                openChartPopout('uv', 'uv', 'UV Index', charts.uv.data.datasets[0].borderColor);
            });
        }
    }

    /**
     * Helper to open the popout chart page with an encoded configuration.
     *
     * Builds a compact per-dataset metadata payload (colours, dashes, widths, fill,
     * plus a `role` hint of average/trend/total from the label) so the popout can
     * mirror the small-card chart visuals, adds an `incomingUnits` hint (from
     * `statusData.unitHints`, `weatherData.unitHints`, or a copy of {@link units}) and
     * the saved theme, then opens `/chart/<type>?config=<json>` in a new tab. Errors
     * are logged, not thrown.
     *
     * @param {string} type Chart key in {@link charts}.
     * @param {string} field Observation field name passed through in the config.
     * @param {string} title Title passed through in the config.
     * @param {string} color Main line colour passed through in the config.
     * @returns {void}
     */
    function openChartPopout(type, field, title, color) {
        try {
            // Build a compact per-dataset metadata payload so the popout can mirror
            // the small-card chart visuals exactly (colors, dashes, widths, fill, etc).
            const chartObj = charts[type];
            const datasetsMeta = [];
            if (chartObj && chartObj.data && Array.isArray(chartObj.data.datasets)) {
                chartObj.data.datasets.forEach(ds => {
                    const meta = {};
                    if (ds.label) meta.label = ds.label;
                    if (ds.borderColor) meta.borderColor = ds.borderColor;
                    if (ds.backgroundColor) meta.backgroundColor = ds.backgroundColor;
                    if (ds.borderDash) meta.borderDash = ds.borderDash;
                    if (ds.borderWidth !== undefined) meta.borderWidth = ds.borderWidth;
                    if (ds.fill !== undefined) meta.fill = ds.fill;
                    if (ds.pointRadius !== undefined) meta.pointRadius = ds.pointRadius;
                    if (ds.tension !== undefined) meta.tension = ds.tension;
                    if (ds.pointStyle !== undefined) meta.pointStyle = ds.pointStyle;
                    if (ds.showLine !== undefined) meta.showLine = ds.showLine;
                    if (ds.stepped !== undefined) meta.stepped = ds.stepped;
                    if (ds.order !== undefined) meta.order = ds.order;
                    if (ds.spanGaps !== undefined) meta.spanGaps = ds.spanGaps;
                    // mark a simple role hint for common lines (Average/Trend/Today Total)
                    if (String(ds.label).toLowerCase().includes('average')) meta.role = 'average';
                    if (String(ds.label).toLowerCase().includes('trend')) meta.role = 'trend';
                    if (String(ds.label).toLowerCase().includes('today') || String(ds.label).toLowerCase().includes('total')) meta.role = 'total';
                    datasetsMeta.push(meta);
                });
            }

            // Build an incomingUnits hint to inform the popout about the units the server/raw data used.
            // Best-effort: if statusData or weatherData expose unit hints, prefer those; otherwise fall back to current `units` (safe default).
            let incomingUnits = null;
            try {
                if (statusData && statusData.unitHints) {
                    incomingUnits = statusData.unitHints;
                } else if (weatherData && weatherData.unitHints) {
                    incomingUnits = weatherData.unitHints;
                } else {
                    // fallback: assume incoming units match the current UI units
                    incomingUnits = Object.assign({}, units);
                }
            } catch (e) {
                incomingUnits = Object.assign({}, units);
            }

            const cfg = { type: type, field: field, title: title, color: color, units: units, incomingUnits: incomingUnits, datasets: datasetsMeta, theme: localStorage.getItem('theme') || 'default' };
            const encoded = encodeURIComponent(JSON.stringify(cfg));
            const url = '/chart/' + type + '?config=' + encoded;
            window.open(url, '_blank');
        } catch(e) {
            debugLog(logLevels.ERROR, 'Failed to open chart popout', e);
        }
    }
    
    // Rain: Purple data → Yellow-green average → Orange 24h total → Cyan window total
    if (charts.rain) {
        ensureDataset(charts.rain, 1);
        ensureDataset(charts.rain, 2);
        ensureDataset(charts.rain, 3);
        charts.rain.data.datasets[1].borderColor = '#66ff66';
        charts.rain.data.datasets[1].backgroundColor = 'rgba(102, 255, 102, 0.2)';
        charts.rain.data.datasets[2].borderColor = '#ff6b35';
        charts.rain.data.datasets[2].backgroundColor = 'rgba(255, 107, 53, 0.1)';
        charts.rain.data.datasets[3].borderColor = '#00d4ff';
        charts.rain.data.datasets[3].backgroundColor = 'rgba(0, 212, 255, 0.1)';
        charts.rain.update('none');
    }
    
    // Pressure: Orange data → Blue average
    if (charts.pressure) {
        ensureDataset(charts.pressure, 1);
        ensureDataset(charts.pressure, 2);
        charts.pressure.data.datasets[1].borderColor = '#4080ff';
        charts.pressure.data.datasets[1].backgroundColor = 'rgba(64, 128, 255, 0.2)';
        charts.pressure.update('none');
    }
    
    // Light: Only has main data, no average line needed
    // (Light naturally goes to zero at night)
    
    // UV: Only has main data, no average line needed  
    // (UV naturally goes to zero at night)
    
    debugLog(logLevels.INFO, '✅ Chart colors forced - complementary pairs applied');
}

/**
 * Show the station elevation in `#tempest-elevation`: feet when the temperature
 * unit is Fahrenheit, metres otherwise, or `--` when unknown.
 * @returns {void}
 */
function updateElevationDisplay() {
    const tempestElevation = document.getElementById('tempest-elevation');
    
    if (!statusData || !statusData.elevation || !tempestElevation) {
        if (tempestElevation) tempestElevation.textContent = '--';
        return;
    }
    
    if (units.temperature === 'fahrenheit') {
        // If using imperial units, show elevation in feet
        const elevationFt = statusData.elevation * 3.28084;
        tempestElevation.textContent = `${Math.round(elevationFt)} ft`;
    } else {
        // If using metric units, show elevation in meters
        tempestElevation.textContent = `${Math.round(statusData.elevation)} m`;
    }
}

/**
 * Refresh the unit labels next to the readings, then the elevation and chart axis labels.
 *
 * Uses {@link prettyUnitLabel} for consistency with popout formatting. The rain and
 * pressure unit elements are rebuilt via `innerHTML` to keep their info icon, so
 * the icon's click listener is re-attached.
 * @returns {void}
 */
function updateUnits() {
    // Use prettyUnitLabel to ensure consistency with popout formatting
    const tempEl = document.getElementById('temperature-unit');
    if (tempEl) tempEl.textContent = prettyUnitLabel('temperature', units.temperature);
    const windEl = document.getElementById('wind-unit');
    if (windEl) windEl.textContent = prettyUnitLabel('wind', units.wind);
    
    // Special handling for rain-unit to preserve the info icon
    const rainUnitElement = document.getElementById('rain-unit');
    const newRainUnitText = units.rain === 'inches' ? 'in ' : 'mm ';
    if (rainUnitElement) {
        const rainInfoIcon = rainUnitElement.querySelector('.info-icon');
        if (rainInfoIcon) {
            rainUnitElement.innerHTML = newRainUnitText + rainInfoIcon.outerHTML;
            // Re-attach click event listener to the rain info icon
            const newRainInfoIcon = rainUnitElement.querySelector('.info-icon');
            if (newRainInfoIcon) {
                newRainInfoIcon.addEventListener('click', function(event) {
                    event.stopPropagation();
                    toggleRainTooltip(event);
                });
            }
        } else {
            // Fallback: just set the text if no info icon
            rainUnitElement.textContent = newRainUnitText.trim();
        }
    }
    
    // Special handling for pressure-unit to preserve the info icon
    const pressureUnitElement = document.getElementById('pressure-unit');
    const newUnitText = units.pressure === 'mb' ? 'mb ' : 'inHg ';
    
    console.log('🔧 updateUnits() - Pressure unit update:', {
        pressureUnitElement: !!pressureUnitElement,
        currentInnerHTML: pressureUnitElement ? pressureUnitElement.innerHTML : 'N/A',
        newUnitText: newUnitText
    });

    if (pressureUnitElement) {
        const infoIcon = pressureUnitElement.querySelector('.info-icon');
        if (infoIcon) {
            // Preserve the info icon by only updating the text node
            pressureUnitElement.innerHTML = newUnitText + infoIcon.outerHTML;
            console.log('🔧 updateUnits() - After setting innerHTML:', pressureUnitElement.innerHTML);

            // Re-attach the event listener to prevent click bubbling
            const newInfoIcon = pressureUnitElement.querySelector('.info-icon');
            if (newInfoIcon) {
                newInfoIcon.addEventListener('click', function(e) {
                    e.stopPropagation();
                    togglePressureTooltip(e);
                });
                console.log('🔧 updateUnits() - Re-attached click event listener to info icon');
            }
        } else {
            // Fallback if no info icon found
            pressureUnitElement.textContent = newUnitText.trim();
            console.log('🔧 updateUnits() - Info icon not found, used textContent fallback');
        }
    }
    
    // Update elevation display with new units
    updateElevationDisplay();
    
    // Update chart labels with new units
    updateChartLabels();
}

/**
 * Toggle one sensor's display unit, persist it, and re-render everything that shows it.
 *
 * Calls {@link updateUnits}, {@link updateDisplay}, {@link refreshForecastDisplay}
 * and {@link recalculateAverages} in that order.
 *
 * @param {string} sensor `'temperature'`, `'wind'`, `'rain'` or `'pressure'`; anything else
 *   only triggers the re-render.
 * @returns {void}
 */
function toggleUnit(sensor) {
    console.log('🚀 toggleUnit() called with sensor:', sensor);
    const oldUnit = units[sensor];
    
    if (sensor === 'temperature') {
        units.temperature = units.temperature === 'celsius' ? 'fahrenheit' : 'celsius';
        localStorage.setItem('temperature-unit', units.temperature);
    } else if (sensor === 'wind') {
        units.wind = units.wind === 'mph' ? 'kph' : 'mph';
        localStorage.setItem('wind-unit', units.wind);
    } else if (sensor === 'rain') {
        units.rain = units.rain === 'inches' ? 'mm' : 'inches';
        localStorage.setItem('rain-unit', units.rain);
    } else if (sensor === 'pressure') {
        units.pressure = units.pressure === 'mb' ? 'inHg' : 'mb';
        localStorage.setItem('pressure-unit', units.pressure);
        console.log('🔄 toggleUnit(pressure) - Unit changed:', {
            oldUnit: oldUnit,
            newUnit: units.pressure
        });
    }
    
    debugLog(logLevels.DEBUG, `Unit toggle for ${sensor}`, {
        oldUnit: oldUnit,
        newUnit: units[sensor],
        allUnits: units
    });
    
    console.log('🔄 toggleUnit() - About to call functions:', {
        sensor: sensor,
        sequence: ['updateUnits()', 'updateDisplay()', 'recalculateAverages()']
    });
    
    updateUnits();
    console.log('🔄 toggleUnit() - updateUnits() completed');
    
    updateDisplay();
    console.log('🔄 toggleUnit() - updateDisplay() completed');
    
    refreshForecastDisplay(); // Update forecast display with new units
    console.log('🔄 toggleUnit() - refreshForecastDisplay() completed');
    
    recalculateAverages(sensor);
    console.log('🔄 toggleUnit() - recalculateAverages() completed');
    console.log('🔄 toggleUnit() - All functions completed');
}

/**
 * Set every dashboard chart's y-axis title to the current units and redraw all charts
 * without animation.
 * @returns {void}
 */
function updateChartLabels() {
    // Update temperature chart Y-axis label
    if (charts.temperature && charts.temperature.options && charts.temperature.options.scales && charts.temperature.options.scales.y) {
        charts.temperature.options.scales.y.title = {
            display: true,
            text: units.temperature === 'celsius' ? '°C' : '°F',
            color: '#444',
            font: { size: 12, weight: '600' }
        };
    }
    
    // Update humidity chart Y-axis label (static, no unit change but included for consistency)
    if (charts.humidity && charts.humidity.options && charts.humidity.options.scales && charts.humidity.options.scales.y) {
        charts.humidity.options.scales.y.title = {
            display: true,
            text: '%',
            color: '#444',
            font: { size: 12, weight: '600' }
        };
    }
    
    // Update wind chart Y-axis label
    if (charts.wind && charts.wind.options && charts.wind.options.scales && charts.wind.options.scales.y) {
        charts.wind.options.scales.y.title = {
            display: true,
            text: units.wind === 'mph' ? 'mph' : 'kph',
            color: '#444',
            font: { size: 12, weight: '600' }
        };
    }
    
    // Update pressure chart Y-axis label
    if (charts.pressure && charts.pressure.options && charts.pressure.options.scales && charts.pressure.options.scales.y) {
        charts.pressure.options.scales.y.title = {
            display: true,
            text: units.pressure === 'mb' ? 'mb' : 'inHg',
            color: '#444',
            font: { size: 12, weight: '600' }
        };
    }
    
    // Update rain chart labels - both Y-axes
    if (charts.rain && charts.rain.options && charts.rain.options.scales) {
        const intensityUnit = units.rain === 'inches' ? 'in/hr' : 'mm/hr';
        const accumUnit = units.rain === 'inches' ? 'in' : 'mm';
        if (charts.rain.options.scales.y) {
            charts.rain.options.scales.y.title = {
                display: true,
                text: `Rain Intensity (${intensityUnit})`,
                color: '#444',
                font: { size: 12, weight: '600' }
            };
        }
        if (charts.rain.options.scales.y1) {
            charts.rain.options.scales.y1.title = {
                display: true,
                text: `Accumulation (${accumUnit})`,
                color: '#444',
                font: { size: 12, weight: '600' }
            };
        }
    }
    
    // Update light chart Y-axis label (static, no unit change but included for consistency)
    if (charts.light && charts.light.options && charts.light.options.scales && charts.light.options.scales.y) {
        charts.light.options.scales.y.title = {
            display: true,
            text: 'lux',
            color: '#444',
            font: { size: 12, weight: '600' }
        };
    }
    
    // Update UV chart Y-axis label (static, no unit change but included for consistency)
    if (charts.uv && charts.uv.options && charts.uv.options.scales && charts.uv.options.scales.y) {
        charts.uv.options.scales.y.title = {
            display: true,
            text: 'UVI',
            color: '#444',
            font: { size: 12, weight: '600' }
        };
    }
    
    // Update all charts
    Object.values(charts).forEach(chart => {
        if (chart && typeof chart.update === 'function') {
            chart.update('none');
        }
    });
    
    debugLog(logLevels.DEBUG, 'Chart labels updated with new units', units);
}

/**
 * Convert a bearing to one of 16 compass points.
 * @param {number} degrees Wind direction in degrees (0 = north).
 * @returns {string} E.g. `'NNE'`.
 */
function degreesToDirection(degrees) {
    const directions = ['N', 'NNE', 'NE', 'ENE', 'E', 'ESE', 'SE', 'SSE', 'S', 'SSW', 'SW', 'WSW', 'W', 'WNW', 'NW', 'NNW'];
    const index = Math.round(degrees / 22.5) % 16;
    return directions[index];
}

/**
 * Map a compass point to an arrow glyph (8 directions).
 * @param {string} direction Compass point from {@link degreesToDirection}.
 * @returns {string} The arrow character; `'↑'` for unknown input.
 */
function updateArrow(direction) {
    const arrows = {
        'N': '↑', 'NNE': '↗', 'NE': '↗', 'ENE': '↗',
        'E': '→', 'ESE': '↘', 'SE': '↘', 'SSE': '↘',
        'S': '↓', 'SSW': '↙', 'SW': '↙', 'WSW': '↙',
        'W': '←', 'WNW': '↖', 'NW': '↖', 'NNW': '↖'
    };
    return arrows[direction] || '↑';
}

/**
 * Convert Celsius to Fahrenheit.
 * @param {number} celsius Temperature in °C.
 * @returns {number} Temperature in °F.
 */
function celsiusToFahrenheit(celsius) {
    return (celsius * 9/5) + 32;
}

/**
 * Convert Fahrenheit to Celsius.
 * @param {number} fahrenheit Temperature in °F.
 * @returns {number} Temperature in °C.
 */
function fahrenheitToCelsius(fahrenheit) {
    return (fahrenheit - 32) * 5/9;
}

/**
 * Convert miles per hour to kilometres per hour.
 * @param {number} mph Speed in miles per hour.
 * @returns {number} Speed in km/h.
 */
function mphToKph(mph) {
    return mph * 1.60934;
}

/**
 * Convert kilometres per hour to miles per hour.
 * @param {number} kph Speed in km/h.
 * @returns {number} Speed in miles per hour.
 */
function kphToMph(kph) {
    return kph / 1.60934;
}

/**
 * Convert inches to millimetres.
 * @param {number} inches Length in inches.
 * @returns {number} Length in millimetres.
 */
function inchesToMm(inches) {
    return inches * 25.4;
}

/**
 * Convert millimetres to inches.
 * @param {number} mm Length in millimetres.
 * @returns {number} Length in inches.
 */
function mmToInches(mm) {
    return mm / 25.4;
}

/**
 * Convert millibars to inches of mercury.
 * @param {number} mb Pressure in millibars.
 * @returns {number} Pressure in inches of mercury.
 */
function mbToInHg(mb) {
    return mb * 0.02953;
}

/**
 * Convert inches of mercury to millibars.
 * @param {number} inHg Pressure in inches of mercury.
 * @returns {number} Pressure in millibars.
 */
function inHgToMb(inHg) {
    return inHg / 0.02953;
}

/**
 * Heat index ("feels like" in heat) using the NOAA Rothfusz regression.
 *
 * Below 80 °F or 40 % relative humidity the formula does not apply and the
 * input temperature is returned unchanged.
 *
 * @param {number} tempC Air temperature in °C.
 * @param {number} humidity Relative humidity in percent.
 * @returns {number} Heat index in °C.
 */
function calculateHeatIndex(tempC, humidity) {
    // Convert temperature to Fahrenheit for calculation
    const tempF = (tempC * 9/5) + 32;
    
    // If conditions don't warrant heat index calculation, return the temperature
    if (tempF < 80 || humidity < 40) {
        debugLog(logLevels.DEBUG, 'Heat index conditions not met, using actual temperature', {
            tempF: tempF,
            humidity: humidity,
            result: tempC
        });
        return tempC; // Return original temperature in Celsius
    }
    
    // Heat Index calculation using the NOAA formula
    const c1 = -42.379, c2 = 2.04901523, c3 = 10.14333127, c4 = -0.22475541;
    const c5 = -0.00683783, c6 = -0.05481717, c7 = 0.00122874, c8 = 0.00085282, c9 = -0.00000199;
    
    // Calculate heat index in Fahrenheit
    const heatIndexF = c1 + (c2 * tempF) + (c3 * humidity) + (c4 * tempF * humidity) +
                     (c5 * tempF * tempF) + (c6 * humidity * humidity) +
                     (c7 * tempF * tempF * humidity) + (c8 * tempF * humidity * humidity) +
                     (c9 * tempF * tempF * humidity * humidity);
    
    // Convert back to Celsius
    const heatIndexC = (heatIndexF - 32) * 5/9;
    
    debugLog(logLevels.DEBUG, 'Heat index calculated', {
        tempC: tempC,
        tempF: tempF,
        humidity: humidity,
        heatIndexF: heatIndexF,
        heatIndexC: heatIndexC
    });
    
    return heatIndexC;
}

/**
 * Describe a Tempest precipitation type code.
 * @param {number} precipType 0 none, 1 rain, 2 hail, 3 rain + hail.
 * @returns {string} `'None'`, `'Rain'`, `'Hail'`, `'Rain + Hail'` or `'Unknown'`.
 */
function getPrecipitationTypeDescription(precipType) {
    switch (precipType) {
        case 0: return 'None';
        case 1: return 'Rain';
        case 2: return 'Hail';
        case 3: return 'Rain + Hail';
        default: return 'Unknown';
    }
}

/**
 * Convert kilometres to miles.
 * @param {number} km Distance in kilometres.
 * @returns {number} Distance in miles.
 */
function kmToMiles(km) {
    return km / 1.60934;
}

/**
 * Convert miles to kilometres.
 * @param {number} miles Distance in miles.
 * @returns {number} Distance in kilometres.
 */
function milesToKm(miles) {
    return miles * 1.60934;
}

/**
 * Mean of the `y` values of a series.
 * @param {Array<ChartPoint>} data Chart points.
 * @returns {number} The average, or 0 for an empty array.
 */
function calculateAverage(data) {
    if (data.length === 0) return 0;
    const sum = data.reduce((acc, point) => acc + point.y, 0);
    const average = sum / data.length;
    
    debugLog(logLevels.DEBUG, `Average calculated for ${data.length} data points`, {
        sum: sum,
        count: data.length,
        average: average
    });
    
    return average;
}

/**
 * Calculate a linear regression trend line.
 *
 * Regresses `y` against the point index (not the timestamp), so uneven sampling
 * is treated as evenly spaced; the original timestamps are kept for x.
 *
 * @param {Array<ChartPoint>} data Chart points in time order.
 * @returns {Array<ChartPoint>} One fitted point per input point, or `[]` for fewer than 2 points.
 */
function calculateTrendLine(data) {
    if (!data || data.length < 2) return [];
    
    // Convert timestamps to numerical values for regression
    const points = data.map((point, index) => ({
        x: index, // Use index as x for linear progression
        y: point.y,
        timestamp: point.x
    }));
    
    const n = points.length;
    const sumX = points.reduce((sum, point) => sum + point.x, 0);
    const sumY = points.reduce((sum, point) => sum + point.y, 0);
    const sumXY = points.reduce((sum, point) => sum + (point.x * point.y), 0);
    const sumXX = points.reduce((sum, point) => sum + (point.x * point.x), 0);
    
    // Calculate slope (m) and intercept (b) for y = mx + b
    const slope = (n * sumXY - sumX * sumY) / (n * sumXX - sumX * sumX);
    const intercept = (sumY - slope * sumX) / n;
    
    // Generate trend line points using original timestamps
    const trendLine = points.map((point, index) => ({
        x: point.timestamp,
        y: slope * index + intercept
    }));
    
    debugLog(logLevels.DEBUG, `Trend line calculated for ${data.length} data points`, {
        slope: slope,
        intercept: intercept,
        firstPoint: trendLine[0],
        lastPoint: trendLine[trendLine.length - 1]
    });
    
    return trendLine;
}

/**
 * Fill dataset 1 of a chart with its average line.
 *
 * Pressure, temperature, humidity and wind (matched by chart identity or by the
 * dataset 0 label) get a single constant average across the full time range (a
 * two-point horizontal line), as summary charts where a global average is more
 * appropriate. Every other chart gets a centred moving average with a window of
 * 10 % of the points, clamped to 5..50. Creates a placeholder dataset 1 if missing.
 *
 * @param {Object} chart Chart.js instance.
 * @param {Array<ChartPoint>} data The main series (normally `chart.data.datasets[0].data`).
 * @returns {void}
 */
function updateAverageLine(chart, data) {
    // Ensure the second dataset exists (used for moving average). Create a minimal
    // placeholder if it's missing so assignments below won't throw.
    if (!chart.data.datasets[1]) {
        chart.data.datasets[1] = chart.data.datasets[1] || { data: [] };
    }

    if (data.length === 0) {
        chart.data.datasets[1].data = [];
        return;
    }

    // For the pressure chart we want a single, constant average line
    // across the full time range (horizontal line). For all other
    // charts use the moving average implementation.
    // Treat these charts as summary charts where a single global average
    // (horizontal line) is more appropriate than a moving average.
    const constantAvgCharts = ['pressure', 'temperature', 'humidity', 'wind'];
    const datasetLabel = (chart && chart.data && chart.data.datasets && chart.data.datasets[0] && String(chart.data.datasets[0].label).toLowerCase()) || '';
    const chartNameFromLabel = constantAvgCharts.find(name => datasetLabel.includes(name));
    const isConstantAvgChart = chartNameFromLabel || (chart === charts.pressure) || (chart === charts.temperature) || (chart === charts.humidity) || (chart === charts.wind);

    if (isConstantAvgChart) {
        // Compute simple average of all y values
        let sum = 0;
        let count = 0;
        for (let i = 0; i < data.length; i++) {
            if (data[i] && typeof data[i].y === 'number') {
                sum += data[i].y;
                count++;
            }
        }

        if (count === 0) {
            chart.data.datasets[1].data = [];
            return;
        }

        const avg = sum / count;

        // Create a horizontal line spanning start..end timestamps
        const firstX = data[0].x;
        const lastX = data[data.length - 1].x;

        chart.data.datasets[1].data = [
            { x: firstX, y: avg },
            { x: lastX, y: avg }
        ];

        debugLog(logLevels.DEBUG, 'Constant average updated', {
            chartLabel: chart.data.datasets[0].label || chartNameFromLabel || 'Chart',
            dataPoints: data.length,
            average: avg
        });
        return;
    }

    // Calculate a moving average with a window of 10% of total data points (minimum 5, maximum 50)
    const windowSize = Math.max(5, Math.min(50, Math.floor(data.length * 0.1)));
    const movingAverageData = [];

    for (let i = 0; i < data.length; i++) {
        // Calculate the range for the moving window
        const start = Math.max(0, i - Math.floor(windowSize / 2));
        const end = Math.min(data.length - 1, i + Math.floor(windowSize / 2));
        
        // Calculate average for the window
        let sum = 0;
        let count = 0;
        for (let j = start; j <= end; j++) {
            if (data[j] && typeof data[j].y === 'number') {
                sum += data[j].y;
                count++;
            }
        }
        
        if (count > 0) {
            movingAverageData.push({
                x: data[i].x,
                y: sum / count
            });
        }
    }

    // Safely assign the moving average points to the second dataset
    chart.data.datasets[1].data = movingAverageData;
    
    debugLog(logLevels.DEBUG, 'Moving average updated', {
        chartLabel: chart.data.datasets[0].label || 'Unknown',
        dataPoints: data.length,
        windowSize: windowSize,
        averagePoints: movingAverageData.length
    });
}

/**
 * Draw the "24-hour total" reference line (dataset 2) on the rain chart.
 *
 * For the rain chart only. A two-point horizontal line at the daily total
 * spanning dataset 0's time range. With dual Y-axes Chart.js auto-scales each
 * axis independently, so no manual scale adjustment is made. Clears the line
 * when the total is missing or there is no main data.
 *
 * @param {Object} chart The rain Chart.js instance.
 * @param {?number} rainDailyTotal Daily rain total in mm (the API unit).
 * @param {Units} units Display units (shadows the global); the total is converted to
 *   inches when `units.rain === 'inches'`.
 * @returns {void}
 */
function update24HourAccumulationLine(chart, rainDailyTotal, units) {
    // For rain chart only - updates the third dataset (index 2) with 24-hour accumulation
    debugLog(logLevels.INFO, 'Updating 24h Rain Line', {
        rainDailyTotal: rainDailyTotal,
        units: units?.rain || 'inches',
        hasDataset2: !!chart.data.datasets[2]
    });
    
    // Also log to console for debugging
    console.log('Rain Daily Total:', rainDailyTotal, 'Type:', typeof rainDailyTotal);
    
    if (!chart.data.datasets[2] || rainDailyTotal === undefined || rainDailyTotal === null) {
        if (chart.data.datasets[2]) {
            chart.data.datasets[2].data = [];
        }
        console.log('WARNING: No rain daily total data available');
        debugLog(logLevels.WARN, '24h Rain Line: No data or dataset - line will be empty');
        return;
    }

    // Convert daily total based on current units. API provides mm; convert to
    // inches only when the UI is set to inches.
    let convertedDailyTotal = rainDailyTotal;
    if (units && units.rain === 'inches') {
        convertedDailyTotal = mmToInches(rainDailyTotal);
    }

    // Create a horizontal line at the daily total level across the current time range
    const mainData = chart.data.datasets[0].data;
    if (mainData.length === 0) {
        chart.data.datasets[2].data = [];
        return;
    }

    // Get the time range from main data
    const startTime = mainData[0].x;
    const endTime = mainData[mainData.length - 1].x;

    // Create horizontal line data points
    const accumulationLineData = [
        { x: startTime, y: convertedDailyTotal },
        { x: endTime, y: convertedDailyTotal }
    ];

    chart.data.datasets[2].data = accumulationLineData;
    
    // Note: With dual Y-axes, we let Chart.js auto-scale each axis independently
    // The left Y-axis (y) scales to rain intensity data
    // The right Y-axis (y1) scales to accumulation and daily total data
    // No manual scale adjustment needed
    
    debugLog(logLevels.DEBUG, '24h accumulation line updated with dual Y-axes auto-scaling', {
        dailyTotal: convertedDailyTotal
    });
    
    debugLog(logLevels.DEBUG, '24h accumulation line updated', {
        originalTotal: rainDailyTotal,
        convertedTotal: convertedDailyTotal,
        unit: units?.rain || 'inches',
        dataPoints: accumulationLineData.length,
        lineData: accumulationLineData
    });
}

/**
 * Updates the accumulated rain line (dataset 3) in the rain chart.
 *
 * This shows cumulative rain over the chart's timeframe: a running sum of
 * dataset 0's `y` values. The incremental values are expected to be calculated
 * by the server to handle decreases in rain_accumulated (from historical data
 * issues or midnight resets), so they are simply summed to get the window total.
 * Does nothing unless datasets 0 and 3 both exist.
 *
 * @param {Object} chart The rain Chart.js instance.
 * @returns {void}
 */
function updateAccumulatedRainLine(chart) {
    if (!chart || !chart.data || !chart.data.datasets[0] || !chart.data.datasets[3]) {
        return;
    }

    const mainData = chart.data.datasets[0].data;
    if (mainData.length === 0) {
        chart.data.datasets[3].data = [];
        return;
    }

    // Calculate cumulative sum of rain data over the visible timeframe
    // The incremental rain values (rainAccum) are calculated by the server to handle
    // decreases in rain_accumulated (from historical data issues or midnight resets).
    // We simply sum these incremental values to get the window total.
    let cumulativeSum = 0;
    const accumulatedData = mainData.map(point => {
        cumulativeSum += (point.y || 0);
        return { x: point.x, y: cumulativeSum };
    });

    chart.data.datasets[3].data = accumulatedData;

    debugLog(logLevels.DEBUG, 'Window total rain line updated', {
        dataPoints: accumulatedData.length,
        windowTotal: cumulativeSum.toFixed(3)
    });
}

/**
 * Temporary test function to simulate the rain issue (for debugging).
 *
 * Sets `weatherData.rainDailyTotal` to 6.724, redraws the 24-hour line and the
 * `#daily-rain-total` text, then restores the original value after 10 seconds.
 * Exposed as `window.testRainChartScaling`. Does nothing until the rain chart and
 * weather data exist.
 * @returns {void}
 */
function testRainChartScaling() {
    if (charts.rain && weatherData) {
        // Simulate the scenario: rain data near 0.0 but daily total is 6.724
        const testDailyTotal = 6.724;
        
        debugLog(logLevels.INFO, 'Testing rain chart scaling with simulated data', {
            currentRainData: weatherData.rainAccum,
            currentDailyTotal: weatherData.rainDailyTotal,
            testDailyTotal: testDailyTotal
        });
        
        // Temporarily override the daily total for testing
        const originalDailyTotal = weatherData.rainDailyTotal;
        weatherData.rainDailyTotal = testDailyTotal;
        
        // Update the 24-hour accumulation line with test data
        update24HourAccumulationLine(charts.rain, testDailyTotal, units);
        charts.rain.update();
        
        // Update the display
        const dailyRainElement = document.getElementById('daily-rain-total');
        if (dailyRainElement) {
            const rainUnit = units.rain === 'inches' ? 'in' : 'mm';
            let displayValue = testDailyTotal;
            if (units.rain === 'mm') {
                displayValue = inchesToMm(testDailyTotal);
            }
            dailyRainElement.textContent = displayValue.toFixed(3) + ' ' + rainUnit;
        }
        
        debugLog(logLevels.INFO, 'Rain chart test applied - check the rain card for scaling');
        
        // Restore original value after 10 seconds
        setTimeout(() => {
            weatherData.rainDailyTotal = originalDailyTotal;
            update24HourAccumulationLine(charts.rain, originalDailyTotal, units);
            charts.rain.update();
            
            if (dailyRainElement) {
                const rainUnit = units.rain === 'inches' ? 'in' : 'mm';
                let displayValue = originalDailyTotal;
                if (units.rain === 'mm') {
                    displayValue = inchesToMm(originalDailyTotal);
                }
                dailyRainElement.textContent = displayValue.toFixed(3) + ' ' + rainUnit;
            }
            
            debugLog(logLevels.INFO, 'Rain chart test restored to original values');
        }, 10000);
    }
}

// Make test function available globally
window.testRainChartScaling = testRainChartScaling;

/**
 * Clean and sort a chart's main series in place.
 *
 * Drops points whose `y` is not a number (or is NaN) or whose `x` is not a valid
 * Date, finite number or parseable date string, converts every `x` to a Date and
 * sorts ascending. Duplicates are kept. Only dataset 0 is touched.
 *
 * @param {Object} chart Chart.js instance.
 * @returns {void}
 */
function validateAndSortChartData(chart) {
    // Validate and sort data for the main dataset
    if (chart.data.datasets[0] && chart.data.datasets[0].data) {
        let data = chart.data.datasets[0].data;

        // Filter out invalid data points and normalize timestamps
        data = data.filter(point => {
            if (!point) return false;
            if (typeof point.y !== 'number' || isNaN(point.y)) return false;

            // Accept Date objects, numeric timestamps, or ISO date strings that parse to valid times
            if (point.x instanceof Date) return !isNaN(point.x.getTime());
            if (typeof point.x === 'number') return Number.isFinite(point.x);
            if (typeof point.x === 'string') return !isNaN(Date.parse(point.x));
            return false;
        }).map(point => {
            // Normalize x to a Date object for consistent sorting/rendering
            return { ...point, x: (point.x instanceof Date) ? point.x : new Date(point.x) };
        });

        // Sort by timestamp (ascending)
        data.sort((a, b) => a.x.getTime() - b.x.getTime());

        // Keep all points (no duplicate removal) but ensure chart receives fully-normalized data
        chart.data.datasets[0].data = data;

        debugLog(logLevels.DEBUG, 'Chart data validated, normalized and sorted', {
            points: data.length,
            first: data[0] || null,
            last: data[data.length - 1] || null
        });
    }
}

/**
 * Fill dataset 2 of a chart with the linear-regression trend line of `data`
 * (see {@link calculateTrendLine}); clears it for fewer than 2 points. Does nothing
 * to the chart if dataset 2 does not exist.
 *
 * @param {Object} chart Chart.js instance (used for the pressure chart).
 * @param {Array<ChartPoint>} data The main series.
 * @returns {void}
 */
function updateTrendLine(chart, data) {
    if (data.length < 2) {
        // Clear trend line if insufficient data
        if (chart.data.datasets[2]) {
            chart.data.datasets[2].data = [];
        }
        return;
    }
    
    const trendLineData = calculateTrendLine(data);
    
    // Ensure trend line dataset exists (dataset index 2)
    if (chart.data.datasets[2]) {
        chart.data.datasets[2].data = trendLineData;
    }
    
    debugLog(logLevels.DEBUG, 'Trend line updated', {
        originalDataPoints: data.length,
        trendLinePoints: trendLineData.length,
        slope: trendLineData.length > 1 ? 
            (trendLineData[trendLineData.length - 1].y - trendLineData[0].y) / (trendLineData.length - 1) : 0
    });
}

/**
 * Describe an illuminance level with a familiar real-world comparison.
 * @param {number} lux Illuminance in lux.
 * @returns {string} E.g. `"Office lighting"`, `"Direct sunlight"`.
 */
function getLuxDescription(lux) {
    let description;
    if (lux <= 0.0001) description = "Moonless, overcast night sky (starlight)";
    else if (lux <= 0.002) description = "Moonless clear night sky with airglow";
    else if (lux <= 0.01) description = "Quarter moon on a clear night";
    else if (lux <= 0.3) description = "Full moon on a clear night";
    else if (lux <= 3.4) description = "Dark limit of civil twilight";
    else if (lux <= 50) description = "Public areas with dark surroundings";
    else if (lux <= 80) description = "Family living room lights";
    else if (lux <= 100) description = "Office building hallway/toilet lighting";
    else if (lux <= 150) description = "Very dark overcast day";
    else if (lux <= 400) description = "Train station platforms";
    else if (lux <= 500) description = "Office lighting";
    else if (lux <= 1000) description = "Sunrise or sunset on a clear day";
    else if (lux <= 25000) description = "Overcast day / Full daylight (not direct sun)";
    else if (lux <= 100000) description = "Direct sunlight";
    else description = "Extremely bright conditions";
    
    debugLog(logLevels.DEBUG, 'Lux description calculated', {
        lux: lux,
        description: description
    });
    
    return description;
}

/**
 * Describe a relative humidity level in comfort terms.
 * @param {number} humidity Relative humidity in percent.
 * @returns {string}
 */
function getHumidityDescription(humidity) {
    let description;
    if (humidity <= 30) description = "Very dry - may cause discomfort";
    else if (humidity <= 40) description = "Dry - comfortable for most people";
    else if (humidity <= 60) description = "Comfortable humidity level";
    else if (humidity <= 70) description = "Slightly humid - still comfortable";
    else if (humidity <= 80) description = "Humid - may feel sticky";
    else description = "Very humid - uncomfortable for most";
    
    debugLog(logLevels.DEBUG, 'Humidity description calculated', {
        humidity: humidity,
        description: description
    });
    
    return description;
}

/**
 * Convert accumulated rainfall to a descriptive category (with emoji), based on
 * standard meteorological classifications and colourful descriptions.
 * @param {number} rainAccumMm Rain amount in millimetres.
 * @returns {string} E.g. `"Light - Soft drizzle 🌦️"`.
 */
function getRainDescription(rainAccumMm) {
    // Convert accumulated rainfall to descriptive categories
    // Based on standard meteorological classifications and colorful descriptions
    let description = "";
    
    if (rainAccumMm <= 0) description = "No precipitation ☀️";
    else if (rainAccumMm <= 0.1) description = "Trace - Barely measurable 🌫️";
    else if (rainAccumMm <= 0.5) description = "Very light - Gentle mist 💧";
    else if (rainAccumMm <= 2) description = "Light - Soft drizzle 🌦️";
    else if (rainAccumMm <= 5) description = "Moderate - Steady shower 🌧️";
    else if (rainAccumMm <= 15) description = "Heavy - Strong downpour ⛈️";
    else if (rainAccumMm <= 30) description = "Very heavy - Intense deluge 🌩️";
    else if (rainAccumMm <= 75) description = "Extreme - Torrential rain ⛈️💦";
    else description = "Cats and dogs - Epic deluge! 🐱🐶💧";
    
    debugLog(logLevels.DEBUG, 'Rain description calculated', {
        rainMm: rainAccumMm,
        description: description
    });
    
    return description;
}

/**
 * Describe a UV index using the standard UV index risk bands (low/moderate/high/very high/extreme).
 * @param {number} uvIndex UV index.
 * @returns {string}
 */
function getUVDescription(uvIndex) {
    let description;
    if (uvIndex <= 2) description = "Low - Low danger from the sun's UV rays";
    else if (uvIndex <= 5) description = "Moderate - Moderate risk of harm from unprotected sun exposure";
    else if (uvIndex <= 7) description = "High - High risk of harm, protection needed";
    else if (uvIndex <= 10) description = "Very High - Very high risk, take extra precautions";
    else description = "Extreme - Extreme risk, take all precautions";
    
    debugLog(logLevels.DEBUG, 'UV description calculated', {
        uvIndex: uvIndex,
        description: description
    });
    
    return description;
}

/**
 * Describe the weather typically associated with a barometric pressure.
 * @param {number} pressure Pressure in the current display unit; converted to mb
 *   first when {@link units}.pressure is `'inHg'`.
 * @returns {string}
 */
function getPressureDescription(pressure) {
    // Convert to mb if needed
    let pressureMb = pressure;
    if (units.pressure === 'inHg') {
        pressureMb = inHgToMb(pressure);
    }

    let description;
    if (pressureMb < 980) description = "Stormy conditions, heavy precipitation likely";
    else if (pressureMb < 990) description = "Rain expected, possible severe weather";
    else if (pressureMb < 1000) description = "Changeable weather, precipitation possible";
    else if (pressureMb < 1010) description = "Fair weather, improving conditions";
    else if (pressureMb < 1020) description = "Clear and dry weather expected";
    else if (pressureMb < 1030) description = "Very dry, stable high pressure system";
    else description = "Exceptionally dry, very stable conditions";
    
    debugLog(logLevels.DEBUG, 'Pressure description calculated', {
        originalPressure: pressure,
        pressureMb: pressureMb,
        units: units.pressure,
        description: description
    });
    
    return description;
}

// Lux tooltip helpers are implemented later; earlier duplicates removed to avoid redeclaration.

/**
 * Toggle the pressure info tooltip.
 *
 * Stops the click from bubbling up to the parent pressure-unit element, and moves
 * the tooltip to `<body>` before showing it to escape the card's stacking context.
 * @param {Event} [event] The click event.
 * @returns {void}
 * @throws {TypeError} If `#pressure-tooltip` is missing.
 */
function togglePressureTooltip(event) {
    // Stop the click from bubbling up to the parent pressure-unit element
    if (event) {
        event.stopPropagation();
    }
    
    const tooltip = document.getElementById('pressure-tooltip');
    
    // Move tooltip to body to escape card stacking context
    if (!tooltip.classList.contains('show')) {
        document.body.appendChild(tooltip);
    }
    
    tooltip.classList.toggle('show');
}

/**
 * Hide the pressure info tooltip.
 * @returns {void}
 * @throws {TypeError} If `#pressure-tooltip` is missing.
 */
function closePressureTooltip() {
    const tooltip = document.getElementById('pressure-tooltip');
    tooltip.classList.remove('show');
}

/**
 * Document click handler that closes the pressure tooltip when the click lands
 * outside both the tooltip and its info icon.
 * @param {MouseEvent} event The click event.
 * @returns {void}
 */
function handlePressureTooltipClickOutside(event) {
    const tooltip = document.getElementById('pressure-tooltip');
    const context = document.getElementById('pressure-context');
    const infoIcon = document.getElementById('pressure-info-icon');

    // If tooltip is visible and click is outside the tooltip and info icon
    if (tooltip && infoIcon && tooltip.classList.contains('show') &&
        !tooltip.contains(event.target) &&
        !infoIcon.contains(event.target)) {
        closePressureTooltip();
    }
}

// Tooltip management functions
/**
 * Toggle the lux info tooltip, moving it to `<body>` before showing it to escape
 * the card's stacking context.
 * @returns {void}
 * @throws {TypeError} If `#lux-tooltip` is missing.
 */
function toggleLuxTooltip() {
    const tooltip = document.getElementById('lux-tooltip');
    
    // Move tooltip to body to escape card stacking context
    if (!tooltip.classList.contains('show')) {
        document.body.appendChild(tooltip);
    }
    
    tooltip.classList.toggle('show');
    debugLog(logLevels.DEBUG, 'Lux tooltip toggled', { visible: tooltip.classList.contains('show') });
}

/**
 * Hide the lux info tooltip.
 * @returns {void}
 * @throws {TypeError} If `#lux-tooltip` is missing.
 */
function closeLuxTooltip() {
    const tooltip = document.getElementById('lux-tooltip');
    tooltip.classList.remove('show');
    debugLog(logLevels.DEBUG, 'Lux tooltip closed');
}

/**
 * Document click handler that closes the lux tooltip when the click lands outside
 * both the tooltip and its info icon.
 * @param {MouseEvent} event The click event.
 * @returns {void}
 */
function handleLuxTooltipClickOutside(event) {
    const tooltip = document.getElementById('lux-tooltip');
    const infoIcon = document.getElementById('lux-info-icon');
    
    if (tooltip && infoIcon && tooltip.classList.contains('show') &&
        !tooltip.contains(event.target) &&
        !infoIcon.contains(event.target)) {
        closeLuxTooltip();
    }
}

// Rain tooltip functions
/**
 * Toggle the rain info tooltip, moving it to `<body>` before showing it to escape
 * the card's stacking context. Logs an error and returns if the tooltip is missing.
 * @param {Event} [event] The click event; its propagation is stopped.
 * @returns {void}
 */
function toggleRainTooltip(event) {
    if (event) {
        event.stopPropagation();
    }
    const tooltip = document.getElementById('rain-tooltip');
    
    if (!tooltip) {
        console.error('Rain tooltip element not found!');
        return;
    }
    
    // Move tooltip to body to escape card stacking context
    if (!tooltip.classList.contains('show')) {
        document.body.appendChild(tooltip);
    }
    
    tooltip.classList.toggle('show');
    debugLog(logLevels.DEBUG, 'Rain tooltip toggled', { visible: tooltip.classList.contains('show') });
}

/**
 * Hide the rain info tooltip.
 * @returns {void}
 * @throws {TypeError} If `#rain-tooltip` is missing.
 */
function closeRainTooltip() {
    const tooltip = document.getElementById('rain-tooltip');
    tooltip.classList.remove('show');
    debugLog(logLevels.DEBUG, 'Rain tooltip closed');
}

/**
 * Document click handler that closes the rain tooltip when the click lands outside
 * both the tooltip and its info icon.
 * @param {MouseEvent} event The click event.
 * @returns {void}
 * @throws {TypeError} If the tooltip is shown but `#rain-info-icon` is missing.
 */
function handleRainTooltipClickOutside(event) {
    const tooltip = document.getElementById('rain-tooltip');
    const infoIcon = document.getElementById('rain-info-icon');
    
    if (tooltip && tooltip.classList.contains('show') && 
        !tooltip.contains(event.target) && !infoIcon.contains(event.target)) {
        closeRainTooltip();
    }
}

// Humidity tooltip functions
/**
 * Toggle the humidity info tooltip, moving it to `<body>` before showing it to
 * escape the card's stacking context.
 * @param {Event} [event] The click event; its propagation is stopped.
 * @returns {void}
 * @throws {TypeError} If `#humidity-tooltip` is missing.
 */
function toggleHumidityTooltip(event) {
    if (event) {
        event.stopPropagation();
    }
    const tooltip = document.getElementById('humidity-tooltip');
    
    // Move tooltip to body to escape card stacking context
    if (!tooltip.classList.contains('show')) {
        document.body.appendChild(tooltip);
    }
    
    tooltip.classList.toggle('show');
    debugLog(logLevels.DEBUG, 'Humidity tooltip toggled', { visible: tooltip.classList.contains('show') });
}

/**
 * Hide the humidity info tooltip.
 * @returns {void}
 * @throws {TypeError} If `#humidity-tooltip` is missing.
 */
function closeHumidityTooltip() {
    const tooltip = document.getElementById('humidity-tooltip');
    tooltip.classList.remove('show');
    debugLog(logLevels.DEBUG, 'Humidity tooltip closed');
}

/**
 * Document click handler that closes the humidity tooltip when the click lands
 * outside both the tooltip and its info icon.
 * @param {MouseEvent} event The click event.
 * @returns {void}
 * @throws {TypeError} If the tooltip is shown but `#humidity-info-icon` is missing.
 */
function handleHumidityTooltipClickOutside(event) {
    const tooltip = document.getElementById('humidity-tooltip');
    const infoIcon = document.getElementById('humidity-info-icon');
    
    if (tooltip && tooltip.classList.contains('show') && 
        !tooltip.contains(event.target) && !infoIcon.contains(event.target)) {
        closeHumidityTooltip();
    }
}

/**
 * Toggle the heat index info tooltip, moving it to `<body>` before showing it to
 * escape the card's stacking context.
 * @returns {void}
 * @throws {TypeError} If `#heat-index-tooltip` is missing.
 */
function toggleHeatIndexTooltip() {
    const tooltip = document.getElementById('heat-index-tooltip');
    
    // Move tooltip to body to escape card stacking context
    if (!tooltip.classList.contains('show')) {
        document.body.appendChild(tooltip);
    }
    
    tooltip.classList.toggle('show');
    debugLog(logLevels.DEBUG, 'Heat index tooltip toggled', { visible: tooltip.classList.contains('show') });
}

/**
 * Hide the heat index info tooltip.
 * @returns {void}
 * @throws {TypeError} If `#heat-index-tooltip` is missing.
 */
function closeHeatIndexTooltip() {
    const tooltip = document.getElementById('heat-index-tooltip');
    tooltip.classList.remove('show');
    debugLog(logLevels.DEBUG, 'Heat index tooltip closed');
}

/**
 * Document click handler that closes the heat index tooltip when the click lands
 * outside both the tooltip and its info icon.
 * @param {MouseEvent} event The click event.
 * @returns {void}
 */
function handleHeatIndexTooltipClickOutside(event) {
    const tooltip = document.getElementById('heat-index-tooltip');
    const infoIcon = document.getElementById('heat-index-info-icon');
    
    if (tooltip && infoIcon && tooltip.classList.contains('show') &&
        !tooltip.contains(event.target) &&
        !infoIcon.contains(event.target)) {
        closeHeatIndexTooltip();
    }
}

/**
 * Toggle the UV info tooltip, moving it to `<body>` before showing it to escape the
 * card's stacking context.
 * @returns {void}
 * @throws {TypeError} If `#uv-tooltip` is missing.
 */
function toggleUVTooltip() {
    const tooltip = document.getElementById('uv-tooltip');
    
    // Move tooltip to body to escape card stacking context
    if (!tooltip.classList.contains('show')) {
        document.body.appendChild(tooltip);
    }
    
    tooltip.classList.toggle('show');
    debugLog(logLevels.DEBUG, 'UV tooltip toggled', { visible: tooltip.classList.contains('show') });
}

/**
 * Hide the UV info tooltip.
 * @returns {void}
 * @throws {TypeError} If `#uv-tooltip` is missing.
 */
function closeUVTooltip() {
    const tooltip = document.getElementById('uv-tooltip');
    tooltip.classList.remove('show');
    debugLog(logLevels.DEBUG, 'UV tooltip closed');
}

/**
 * Document click handler that closes the UV tooltip when the click lands outside
 * both the tooltip and its info icon.
 * @param {MouseEvent} event The click event.
 * @returns {void}
 */
function handleUVTooltipClickOutside(event) {
    const tooltip = document.getElementById('uv-tooltip');
    const infoIcon = document.getElementById('uv-info-icon');
    
    if (tooltip && infoIcon && tooltip.classList.contains('show') &&
        !tooltip.contains(event.target) &&
        !infoIcon.contains(event.target)) {
        closeUVTooltip();
    }
}

/**
 * Render the current {@link weatherData} into the dashboard cards.
 *
 * Covers temperature, humidity and heat index, wind (speed, gust text, direction
 * and arrow), rain (incremental and daily total, description, precipitation type),
 * lightning, pressure (value, server-provided condition/trend/forecast, sea-level
 * pressure), light and UV, the last-update time, battery voltage and the
 * observation count. Skipped on pages without `#temperature` (the chart popout)
 * and when no weather data has been fetched yet.
 * @returns {void}
 * @throws {TypeError} If a required dashboard element or numeric field (e.g.
 *   `humidity`, `windDirection`, `illuminance`) is missing; {@link fetchWeather}
 *   catches this.
 */
function updateDisplay() {
    // If the page does not include the main dashboard elements (for example
    // the chart popout page which only includes a single canvas), skip the
    // full display update to avoid null dereferences when elements like
    // `#temperature` are absent.
    if (!document.getElementById('temperature')) {
        debugLog(logLevels.WARN, 'updateDisplay: dashboard elements not present on this page - skipping full display update');
        return;
    }

    if (!weatherData) {
        debugLog(logLevels.WARN, 'updateDisplay called but no weatherData available');
        return;
    }
    
    debugLog(logLevels.DEBUG, 'Updating display with weather data', weatherData);

    // Temperature calculation and display
    document.getElementById('temperature').textContent = formatTemperature(weatherData.temperature);
    debugLog(logLevels.DEBUG, 'Temperature updated', {
        original: weatherData.temperature,
        formatted: formatTemperature(weatherData.temperature),
        unit: units.temperature
    });

    // Humidity and heat index
    document.getElementById('humidity').textContent = weatherData.humidity.toFixed(1);
    document.getElementById('humidity-description').textContent = getHumidityDescription(weatherData.humidity);
    
    // Calculate and display heat index
    const heatIndexC = calculateHeatIndex(weatherData.temperature, weatherData.humidity);
    const heatIndexElement = document.getElementById('heat-index');
    if (heatIndexElement) {
        heatIndexElement.textContent = formatTemperature(heatIndexC);
    }
    debugLog(logLevels.DEBUG, 'Heat index calculated and displayed', {
        heatIndexC: heatIndexC,
        formatted: formatTemperature(heatIndexC),
        unit: units.temperature
    });

    // Wind data
    document.getElementById('wind-speed').textContent = formatWindSpeed(weatherData.windSpeed);

    // Define converted wind variables for logging and display consistency
    let windSpeed = typeof weatherData.windSpeed === 'number' ? weatherData.windSpeed : 0;
    let windGust = typeof weatherData.windGust === 'number' ? weatherData.windGust : 0;
    if (units.wind === 'kph') {
        windSpeed = mphToKph(windSpeed);
        windGust = mphToKph(windGust);
    }

    // Wind gust information
    const windUnit = units.wind === 'kph' ? 'kph' : 'mph';
    if (weatherData.windGust > weatherData.windSpeed) {
        document.getElementById('wind-gust-info').textContent = `Winds gusting to ${formatWindSpeed(weatherData.windGust)}`;
    } else if (weatherData.windGust > 0) {
        document.getElementById('wind-gust-info').textContent = `Gusts up to ${formatWindSpeed(weatherData.windGust)}`;
    } else {
        document.getElementById('wind-gust-info').textContent = 'No gusts detected';
    }

    const direction = degreesToDirection(weatherData.windDirection);
    document.getElementById('wind-direction').textContent = direction + ' (' + weatherData.windDirection.toFixed(0) + '°)';
    document.getElementById('wind-arrow').textContent = updateArrow(direction);
    debugLog(logLevels.DEBUG, 'Wind data updated', {
        originalSpeed: weatherData.windSpeed,
        convertedSpeed: windSpeed,
        originalGust: weatherData.windGust,
        convertedGust: windGust,
        direction: weatherData.windDirection,
        directionText: direction,
        unit: units.wind
    });

    // Rain data
    // Prepare converted values for rain and wind to avoid referencing undefined variables
    // Server provides rain values as inches (incremental). Convert to millimeters for
    // Rain data from backend is now in mm (native units)
    // formatRain and getRainDescription expect mm input
    const rainMm = typeof weatherData.rainAccum === 'number' ? weatherData.rainAccum : 0;

    // Display current incremental rain (formatRain expects mm input)
    document.getElementById('rain').textContent = formatRain(rainMm);

    // Display daily rain total (also in mm from backend)
    const dailyRainElement = document.getElementById('daily-rain-total');
    const dailyRainMm = typeof weatherData.rainDailyTotal === 'number' ? weatherData.rainDailyTotal : 0;
    if (dailyRainElement) {
        // formatRain expects mm and handles unit conversion internally
        dailyRainElement.textContent = formatRain(dailyRainMm || 0);
    }

    // Display rain description based on current accumulated rain (in mm)
    const rainDescElement = document.getElementById('rain-description');
    if (rainDescElement) {
        rainDescElement.textContent = getRainDescription(rainMm);
    }

    
    // Precipitation type data
    const precipitationTypeElement = document.getElementById('precipitation-type');
    if (precipitationTypeElement) {
        const precipType = weatherData.precipitationType || 0;
        precipitationTypeElement.textContent = getPrecipitationTypeDescription(precipType);
        
        debugLog(logLevels.DEBUG, 'Updated precipitation type', {
            precipitationType: precipType,
            description: getPrecipitationTypeDescription(precipType)
        });
    }
    
    // Lightning data
    const lightningCountElement = document.getElementById('lightning-count');
    const lightningDistanceElement = document.getElementById('lightning-distance');
    const lightningDistanceUnitElement = document.getElementById('lightning-distance-unit');
    
    if (lightningCountElement) {
        lightningCountElement.textContent = weatherData.lightningStrikeCount || 0;
    }
    
    if (lightningDistanceElement && lightningDistanceUnitElement) {
        let lightningDistance = weatherData.lightningStrikeAvg || 0;
        if (units.rain === 'inches') {
            lightningDistance = kmToMiles(lightningDistance);
            lightningDistanceUnitElement.textContent = 'mi';
        } else {
            lightningDistanceUnitElement.textContent = 'km';
        }
        lightningDistanceElement.textContent = lightningDistance.toFixed(1);
    }
    
    debugLog(logLevels.DEBUG, 'Rain and lightning data updated', {
        originalRain: weatherData.rainAccum,
        convertedRain: rainMm,
        originalDailyRain: weatherData.rainDailyTotal,
        convertedDailyRain: dailyRainMm,
        rainUnit: units.rain,
        rainDescription: getRainDescription(rainMm),
        lightningCount: weatherData.lightningStrikeCount,
        lightningDistance: weatherData.lightningStrikeAvg
    });

    let pressure = weatherData.pressure;
    if (units.pressure === 'inHg') {
        pressure = mbToInHg(pressure);
    }
    document.getElementById('pressure').textContent = formatPressure(weatherData.pressure);
    
    // Use server-provided pressure analysis - AGGRESSIVE DEBUGGING (v3.0)
    const apiCondition = weatherData.pressure_condition;
    const apiTrend = weatherData.pressure_trend;
    const apiForecast = weatherData.weather_forecast;
    
    console.log('� AGGRESSIVE PRESSURE DEBUG:', {
        'Raw API Object': weatherData,
        'Extracted apiCondition': apiCondition,
        'Extracted apiTrend': apiTrend, 
        'Extracted apiForecast': apiForecast,
        'About to set pressure-condition to': apiCondition,
        'About to set pressure-trend to': apiTrend,
        'About to set pressure-forecast to': apiForecast
    });
    
    const conditionElement = document.getElementById('pressure-condition');
    const trendElement = document.getElementById('pressure-trend');
    const forecastElement = document.getElementById('pressure-forecast');
    const seaLevelElement = document.getElementById('pressure-sea-level');
    
    if (conditionElement) conditionElement.textContent = apiCondition || '--';
    if (trendElement) trendElement.textContent = apiTrend || '--';  
    if (forecastElement) forecastElement.textContent = apiForecast || '--';
    
    // Display sea level pressure with unit conversion
    if (seaLevelElement && weatherData.seaLevelPressure) {
        let seaLevelPressure = weatherData.seaLevelPressure;
        let pressureUnit = 'mb';
        
        if (units.pressure === 'inHg') {
            seaLevelPressure = mbToInHg(seaLevelPressure);
            pressureUnit = 'inHg';
        }
        
        seaLevelElement.textContent = `${Math.round(seaLevelPressure)} ${pressureUnit}`;
    } else if (seaLevelElement) {
        seaLevelElement.textContent = '--';
    }
    
    console.log('✅ AFTER SETTING:', {
        'pressure-condition element text': conditionElement ? conditionElement.textContent : 'NOT FOUND',
        'pressure-trend element text': trendElement ? trendElement.textContent : 'NOT FOUND',
        'pressure-forecast element text': forecastElement ? forecastElement.textContent : 'NOT FOUND'
    });

    // Light and UV data
    document.getElementById('illuminance').textContent = weatherData.illuminance.toFixed(0);
    document.getElementById('lux-description').textContent = getLuxDescription(weatherData.illuminance);
    
    const uvElement = document.getElementById('uv-index');
    const uvDescElement = document.getElementById('uv-description');
    if (uvElement) uvElement.textContent = Math.round(weatherData.uv);
    if (uvDescElement) uvDescElement.textContent = getUVDescription(weatherData.uv);
    
    debugLog(logLevels.DEBUG, 'Light and UV data updated', {
        illuminance: weatherData.illuminance,
        luxDescription: getLuxDescription(weatherData.illuminance),
        uv: weatherData.uv,
        uvDescription: getUVDescription(weatherData.uv)
    });

    // Last update timestamp
    const lastUpdateText = new Date(weatherData.lastUpdate).toLocaleString('en-US', {
        year: 'numeric',
        month: '2-digit', 
        day: '2-digit',
        hour: '2-digit',
        minute: '2-digit',
        hour12: false
    });
    document.getElementById('last-update').textContent = lastUpdateText;
    
    // Battery level (update from current weather data)
    const tempestBatteryFromWeather = document.getElementById('tempest-battery');
    if (tempestBatteryFromWeather && weatherData.battery !== undefined) {
        if (weatherData.battery === 0) {
            tempestBatteryFromWeather.textContent = 'N/A';
            debugLog(logLevels.DEBUG, '🔋 Battery data not available from API (returned 0)');
        } else {
            tempestBatteryFromWeather.textContent = `${weatherData.battery.toFixed(1)}V`;
            debugLog(logLevels.DEBUG, '🔋 Battery updated from weather data:', `${weatherData.battery.toFixed(1)}V`);
        }
    }
    
    // Update data count (real-time update as data is collected)
    const tempestDataCount = document.getElementById('tempest-data-count');
    if (tempestDataCount && weatherData.observationCount !== undefined && weatherData.maxHistorySize !== undefined) {
        tempestDataCount.textContent = weatherData.observationCount > 0 ? `${weatherData.observationCount}/${weatherData.maxHistorySize}` : '0';
        debugLog(logLevels.DEBUG, '📊 Data count updated:', `${weatherData.observationCount}/${weatherData.maxHistorySize}`);
    }
    
    debugLog(logLevels.INFO, 'Display update completed', {
        lastUpdate: weatherData.lastUpdate,
        formattedTime: lastUpdateText
    });
}

/**
 * Append the latest {@link weatherData} reading (timestamped now) to every live chart.
 *
 * Converts each value to the display units, trims series to {@link maxDataPoints},
 * and refreshes average lines (all charts), the pressure trend line and the rain
 * accumulation series, which is integrated from the rain rate and then offset so
 * its last point equals the API's `rainDailyTotal`. On the popout page it updates
 * `charts.popout` instead, including the average line, the pressure trend, and for
 * rain a window accumulation plus a midnight-aware "today total" step line. Each
 * chart's `update()` is wrapped in try/catch.
 * @returns {void}
 */
function updateCharts() {
    console.log('🚀 DEBUG: updateCharts() called', { weatherData: weatherData });
    
    if (!weatherData) {
        console.warn('⚠️ DEBUG: updateCharts called but no weatherData available');
        debugLog(logLevels.WARN, 'updateCharts called but no weatherData available');
        return;
    }
    
    console.log('🚀 DEBUG: Starting chart updates with data:', weatherData);
    debugLog(logLevels.DEBUG, 'Starting chart updates');
    
    // Use current time for live data updates - this creates real-time progression
    const now = new Date();
    
    // Debug generated weather data freshness
    console.log('📊 CHART UPDATE DEBUG:', {
        currentTime: now.toISOString(),
        weatherLastUpdate: weatherData.lastUpdate,
        temperature: weatherData.temperature,
        timeDiff: now.getTime() - new Date(weatherData.lastUpdate).getTime()
    });

    // Temperature chart (defensive)
    let tempValue = (typeof weatherData.temperature === 'number' && Number.isFinite(weatherData.temperature)) ? weatherData.temperature : 0;
    if (units.temperature === 'fahrenheit') {
        tempValue = celsiusToFahrenheit(tempValue);
    }
    if (charts.temperature && charts.temperature.data && charts.temperature.data.datasets && charts.temperature.data.datasets[0]) {
        charts.temperature.data.datasets[0].data.push({ x: now, y: tempValue });
        if (charts.temperature.data.datasets[0].data.length > maxDataPoints) charts.temperature.data.datasets[0].data.shift();
        const tempAvg = calculateAverage(charts.temperature.data.datasets[0].data);
        validateAndSortChartData(charts.temperature);
        updateAverageLine(charts.temperature, charts.temperature.data.datasets[0].data);
        charts.temperature.options.scales.y.title = {
            display: true,
            text: units.temperature === 'celsius' ? '°C' : '°F'
        };
        try { charts.temperature.update(); } catch (e) { debugLog(logLevels.ERROR, 'Temperature chart update failed', { error: e.message }); }
        debugLog(logLevels.DEBUG, 'Temperature chart updated', { dataPoints: charts.temperature.data.datasets[0].data.length, currentValue: tempValue, average: tempAvg });
    }

    // Humidity chart (defensive)
    const humidityValue = (typeof weatherData.humidity === 'number' && Number.isFinite(weatherData.humidity)) ? weatherData.humidity : 0;
    if (charts.humidity && charts.humidity.data && charts.humidity.data.datasets && charts.humidity.data.datasets[0]) {
        charts.humidity.data.datasets[0].data.push({ x: now, y: humidityValue });
        if (charts.humidity.data.datasets[0].data.length > maxDataPoints) charts.humidity.data.datasets[0].data.shift();
        const humidityAvg = calculateAverage(charts.humidity.data.datasets[0].data);
        updateAverageLine(charts.humidity, charts.humidity.data.datasets[0].data);
        charts.humidity.options.scales.y.title = { display: true, text: '%' };
        try { charts.humidity.update(); } catch (e) { debugLog(logLevels.ERROR, 'Humidity chart update failed', { error: e.message }); }
    }

    // Wind chart (defensive)
    let windValue = (typeof weatherData.windSpeed === 'number' && Number.isFinite(weatherData.windSpeed)) ? weatherData.windSpeed : 0;
    if (units.wind === 'kph') windValue = mphToKph(windValue);
    if (charts.wind && charts.wind.data && charts.wind.data.datasets && charts.wind.data.datasets[0]) {
        charts.wind.data.datasets[0].data.push({ x: now, y: windValue });
        if (charts.wind.data.datasets[0].data.length > maxDataPoints) charts.wind.data.datasets[0].data.shift();
        const windAvg = calculateAverage(charts.wind.data.datasets[0].data);
        updateAverageLine(charts.wind, charts.wind.data.datasets[0].data);
        charts.wind.options.scales.y.title = { display: true, text: units.wind === 'mph' ? 'mph' : 'kph' };
        try { charts.wind.update(); } catch (e) { debugLog(logLevels.ERROR, 'Wind chart update failed', { error: e.message }); }
    }

    // Rain chart - now shows intensity (rate) and accumulation
    // Rain rate comes from server in mm/hr, convert to inches/hr if user prefers inches
    let rainRate = (typeof weatherData.rainRate === 'number' && Number.isFinite(weatherData.rainRate)) ? weatherData.rainRate : 0;
    // Convert to inches/hr if user prefers inches
    if (units.rain === 'inches') {
        rainRate = mmToInches(rainRate);
    }
    if (charts.rain && charts.rain.data && charts.rain.data.datasets && charts.rain.data.datasets[0] && charts.rain.data.datasets[1]) {
        // Dataset 0: Rain Intensity (rate in user's preferred units)
        charts.rain.data.datasets[0].data.push({ x: now, y: rainRate });
        if (charts.rain.data.datasets[0].data.length > maxDataPoints) charts.rain.data.datasets[0].data.shift();
        
        // Dataset 1: Rain Accumulation (show cumulative rain over time)
        // Compute raw accumulation from the available data (in display units),
        // then, if the API provides a daily total, offset the series so the
        // final value matches the API's `rainDailyTotal` (converted to display units).
        let cumulativeRain = 0;
        const accumulationData = [];
        for (let i = 0; i < charts.rain.data.datasets[0].data.length; i++) {
            const point = charts.rain.data.datasets[0].data[i];
            if (i > 0) {
                const prevPoint = charts.rain.data.datasets[0].data[i - 1];
                const timeDiffHours = (point.x - prevPoint.x) / (1000 * 60 * 60); // Convert ms to hours
                // point.y is already in user's preferred units (mm/hr or in/hr)
                cumulativeRain += point.y * timeDiffHours; // rate * time = accumulation
            }
            accumulationData.push({ x: point.x, y: cumulativeRain });
        }

        // If API provides a rainDailyTotal (mm), convert to display units and
        // offset the accumulation series so the last point equals that total.
        if (weatherData && typeof weatherData.rainDailyTotal === 'number' && accumulationData.length > 0) {
            const apiDaily = (units.rain === 'inches') ? mmToInches(weatherData.rainDailyTotal) : weatherData.rainDailyTotal;
            const rawFinal = accumulationData[accumulationData.length - 1].y || 0;
            const offset = apiDaily - rawFinal;
            if (Math.abs(offset) > 1e-9) {
                for (let i = 0; i < accumulationData.length; i++) {
                    accumulationData[i].y = accumulationData[i].y + offset;
                }
            }
        }

        charts.rain.data.datasets[1].data = accumulationData;
        
        try { charts.rain.update(); } catch (e) { debugLog(logLevels.ERROR, 'Rain chart update failed', { error: e.message }); }
    }

    // Pressure chart (defensive)
    let pressureValue = (typeof weatherData.pressure === 'number' && Number.isFinite(weatherData.pressure)) ? weatherData.pressure : 0;
    if (units.pressure === 'inHg') pressureValue = mbToInHg(pressureValue);
    if (charts.pressure && charts.pressure.data && charts.pressure.data.datasets && charts.pressure.data.datasets[0]) {
        charts.pressure.data.datasets[0].data.push({ x: now, y: pressureValue });
        if (charts.pressure.data.datasets[0].data.length > maxDataPoints) charts.pressure.data.datasets[0].data.shift();
        const pressureAvg = calculateAverage(charts.pressure.data.datasets[0].data);
        updateAverageLine(charts.pressure, charts.pressure.data.datasets[0].data);
        updateTrendLine(charts.pressure, charts.pressure.data.datasets[0].data);
        charts.pressure.options.scales.y.title = { display: true, text: units.pressure === 'mb' ? 'mb' : 'inHg' };
        try { charts.pressure.update(); } catch (e) { debugLog(logLevels.ERROR, 'Pressure chart update failed', { error: e.message }); }
    }

    // Light chart (defensive)
    const illuminanceValue = (typeof weatherData.illuminance === 'number' && Number.isFinite(weatherData.illuminance)) ? weatherData.illuminance : 0;
    if (charts.light && charts.light.data && charts.light.data.datasets && charts.light.data.datasets[0]) {
        charts.light.data.datasets[0].data.push({ x: now, y: illuminanceValue });
        if (charts.light.data.datasets[0].data.length > maxDataPoints) charts.light.data.datasets[0].data.shift();
        const lightAvg = calculateAverage(charts.light.data.datasets[0].data);
        updateAverageLine(charts.light, charts.light.data.datasets[0].data);
        charts.light.options.scales.y.title = { display: true, text: 'lux' };
        try { if (charts.light) validateAndSortChartData(charts.light); } catch (e) { debugLog(logLevels.ERROR, 'Failed to validate/sort light chart data', { error: e.message }); }
        try { charts.light.update(); } catch (e) { debugLog(logLevels.ERROR, 'Light chart update failed', { error: e.message }); }
        debugLog(logLevels.DEBUG, 'Light chart updated', { dataPoints: charts.light.data.datasets[0].data.length, currentValue: illuminanceValue, average: lightAvg });
    }

    // UV chart - only update if it exists
    if (charts.uv && charts.uv.data && charts.uv.data.datasets && charts.uv.data.datasets[0]) {
        const uvValue = (typeof weatherData.uv === 'number' && Number.isFinite(weatherData.uv)) ? weatherData.uv : 0;
        charts.uv.data.datasets[0].data.push({ x: now, y: uvValue });
        if (charts.uv.data.datasets[0].data.length > maxDataPoints) charts.uv.data.datasets[0].data.shift();
        const uvAvg = calculateAverage(charts.uv.data.datasets[0].data);
        updateAverageLine(charts.uv, charts.uv.data.datasets[0].data);
        charts.uv.options.scales.y.title = { display: true, text: 'UVI' };
        try { validateAndSortChartData(charts.uv); } catch (e) { debugLog(logLevels.ERROR, 'Failed to validate/sort UV chart data', { error: e.message }); }
        try { charts.uv.update(); } catch (e) { debugLog(logLevels.ERROR, 'UV chart update failed', { error: e.message }); }
        debugLog(logLevels.DEBUG, 'UV chart updated', { dataPoints: charts.uv.data.datasets[0].data.length, currentValue: uvValue, average: uvAvg });
    } else {
        debugLog(logLevels.DEBUG, 'UV chart not available, skipping UV update');
    }

    // Update popout chart if present (for expanded chart views)
    if (charts.popout && charts.popout.data && charts.popout.data.datasets && charts.popout.data.datasets.length > 0) {
        // Determine which data to show based on the chart type stored during initialization
        let popoutValue = 0;
        const chartType = charts.popoutType || 'temperature';
        // Main data is the first dataset in popout charts (drawn underneath)
        const datasetIndex = 0;
        
        switch(chartType) {
            case 'temperature':
                popoutValue = tempValue;
                break;
            case 'humidity':
                popoutValue = humidityValue;
                break;
            case 'wind':
                popoutValue = windValue;
                break;
            case 'rain':
                // Use rain rate (intensity) for rain popup charts
                // Backend sends in mm/hr, convert to inches/hr if user prefers inches
                popoutValue = (typeof weatherData.rainRate === 'number' && Number.isFinite(weatherData.rainRate)) ? weatherData.rainRate : 0;
                if (units.rain === 'inches') {
                    popoutValue = mmToInches(popoutValue); // Convert mm/hr to in/hr
                }
                break;
            case 'pressure':
                popoutValue = pressureValue;
                break;
            case 'light':
                popoutValue = illuminanceValue;
                break;
            case 'uv':
                const uvVal = (typeof weatherData.uv === 'number' && Number.isFinite(weatherData.uv)) ? weatherData.uv : 0;
                popoutValue = uvVal;
                break;
            default:
                popoutValue = tempValue;
        }
        
        charts.popout.data.datasets[datasetIndex].data.push({ x: now, y: popoutValue });
        if (charts.popout.data.datasets[datasetIndex].data.length > maxDataPoints) {
            charts.popout.data.datasets[datasetIndex].data.shift();
        }
        
        // Update average line (dataset[1]) - skip for light and UV
        const mainData = charts.popout.data.datasets[datasetIndex].data;
        if (mainData.length > 0) {
            const firstX = mainData[0].x;
            const lastX = mainData[mainData.length - 1].x;
            
            // Calculate and update average line for charts that have one
            if (chartType !== 'light' && chartType !== 'uv') {
                let sum = 0;
                let count = 0;
                for (let i = 0; i < mainData.length; i++) {
                    if (mainData[i] && typeof mainData[i].y === 'number') {
                        sum += mainData[i].y;
                        count++;
                    }
                }
                const avg = count > 0 ? sum / count : 0;
                if (charts.popout.data.datasets[1]) {
                    charts.popout.data.datasets[1].data = [
                        { x: firstX, y: avg },
                        { x: lastX, y: avg }
                    ];
                }
            }
            
            // Update trend line for pressure (dataset[2])
            // popout dataset order: main(0), average(1), trend/accum/total (2+)
            if (chartType === 'pressure' && charts.popout.data.datasets[2]) {
                const trendData = calculateTrendLine(mainData);
                charts.popout.data.datasets[2].data = trendData;
            }
            
            // Update accumulation and today total lines for rain (datasets 1 and 2)
            // Rain intensity is dataset 0, accumulation is dataset 1, today total is dataset 2
            if (chartType === 'rain') {
                // Calculate accumulation from intensity (rate * time) and a midnight-aware
                // per-point "today total" (step) so previous days are shown as a flat
                // total and the current day increases from midnight to now.
                if (mainData.length > 0) {
                    /**
                     * Helper: local date key (YYYY-MM-DD) for grouping rain by calendar day.
                     * @param {Date|number} ts Timestamp.
                     * @returns {string}
                     */
                    const localDateKey = ts => {
                        const d = new Date(ts);
                        const y = d.getFullYear();
                        const m = String(d.getMonth() + 1).padStart(2, '0');
                        const day = String(d.getDate()).padStart(2, '0');
                        return `${y}-${m}-${day}`;
                    };

                    // Running totals per day and cumulative since midnight per point
                    const runningByDay = {};
                    const cumulativeSinceMidnight = new Array(mainData.length).fill(0);
                    // Accumulation across entire window (fallback dataset)
                    let cumulativeWindow = 0;
                    const accumulationData = [];

                    for (let i = 0; i < mainData.length; i++) {
                        if (i === 0) {
                            // initialize
                            cumulativeSinceMidnight[i] = 0;
                            accumulationData.push({ x: mainData[i].x, y: units.rain === 'inches' ? mmToInches(0) : 0 });
                            continue;
                        }

                        const t0 = mainData[i - 1].x;
                        const t1 = mainData[i].x;
                        const intensity = mainData[i].y || 0; // may be mm/hr or inches/hr depending on units
                        const intensityMm = (units.rain === 'inches') ? inchesToMm(intensity) : intensity;

                        // Walk the interval and split across midnights
                        let segStart = t0;
                        while (segStart < t1) {
                            const segStartDate = new Date(segStart);
                            const nextMidnight = new Date(segStartDate);
                            nextMidnight.setHours(24, 0, 0, 0);
                            const segEnd = Math.min(t1, nextMidnight.getTime());
                            const segHours = (segEnd - segStart) / (1000 * 60 * 60);
                            const delta = intensityMm * segHours; // mm
                            const dayKey = localDateKey(segStart);
                            runningByDay[dayKey] = (runningByDay[dayKey] || 0) + delta;
                            // advance
                            segStart = segEnd;
                        }

                        // cumulative since midnight for this point is the running total for the point's day
                        const thisDayKey = localDateKey(mainData[i].x);
                        cumulativeSinceMidnight[i] = runningByDay[thisDayKey] || 0;

                        // window cumulative (no resets) in mm
                        cumulativeWindow += intensityMm * ((t1 - t0) / (1000 * 60 * 60));
                        const displayAccum = (units.rain === 'inches') ? mmToInches(cumulativeWindow) : cumulativeWindow;
                        accumulationData.push({ x: mainData[i].x, y: displayAccum });
                    }

                    // Set accumulation dataset (find the dataset labeled 'Accumulation')
                    const accumDs = charts.popout.data.datasets.find(ds => ds.label && ds.label.toLowerCase().includes('accumul'));
                    if (accumDs) {
                        accumDs.data = accumulationData;
                    }

                    // Build today-total (step) dataset: for points on previous days use that day's final total,
                    // for points on the current day use cumulative since midnight at that timestamp.
                    const todayKey = localDateKey(Date.now());
                    const todayTotalDs = charts.popout.data.datasets.find(ds => ds.label && ds.label.toLowerCase().includes('today'));
                    if (todayTotalDs) {
                        const todayData = [];
                        for (let i = 0; i < mainData.length; i++) {
                            const pk = localDateKey(mainData[i].x);
                            let yVal = 0;
                            if (pk === todayKey) {
                                // current day - use running cumulative
                                yVal = cumulativeSinceMidnight[i] || 0;
                            } else {
                                // previous day - show the day's total (may be available in runningByDay)
                                yVal = runningByDay[pk] || 0;
                            }
                            if (units.rain === 'inches') yVal = mmToInches(yVal);
                            todayData.push({ x: mainData[i].x, y: yVal });
                        }
                        todayTotalDs.data = todayData;
                        // Render today-total as a stepped line for clarity
                        todayTotalDs.stepped = true;
                    }
                }
            }
        }
        
        try { 
            charts.popout.update(); 
            debugLog(logLevels.DEBUG, 'Popout chart updated', { 
                type: chartType, 
                value: popoutValue,
                dataPoints: mainData.length,
                avgPoints: charts.popout.data.datasets[1] ? charts.popout.data.datasets[1].data.length : 0,
                trendPoints: (chartType === 'pressure' && charts.popout.data.datasets[2]) ? charts.popout.data.datasets[2].data.length : 0
            });
        } catch (e) { 
            debugLog(logLevels.ERROR, 'Popout chart update failed', { error: e.message }); 
        }
    }

    debugLog(logLevels.INFO, 'All charts updated successfully');
}

/**
 * Convert the stored series of the one sensor whose unit just changed, then refresh
 * its derived lines.
 *
 * Only the sensor that actually changed units is converted, which prevents
 * double-conversion. Also rewrites the pressure condition/trend/forecast text and
 * the `#daily-rain-total` text, and always resets the wind chart's axis title.
 *
 * @param {string} changedSensor `'temperature'`, `'wind'`, `'rain'` or `'pressure'`.
 * @returns {void}
 * @throws {TypeError} If the matching chart has not been created yet.
 */
function recalculateAverages(changedSensor) {
    // Only recalculate data for the sensor that actually changed units
    // This prevents double-conversion issues
    
    // Recalculate temperature data and average
    if (changedSensor === 'temperature' && charts.temperature.data.datasets[0].data.length > 0) {
        charts.temperature.data.datasets[0].data.forEach(point => {
            if (units.temperature === 'fahrenheit') {
                point.y = celsiusToFahrenheit(point.y);
            } else {
                point.y = fahrenheitToCelsius(point.y);
            }
        });
        const tempAvg = calculateAverage(charts.temperature.data.datasets[0].data);
        updateAverageLine(charts.temperature, charts.temperature.data.datasets[0].data);
        charts.temperature.update();
    }

    // Recalculate wind data and average
    if (changedSensor === 'wind' && charts.wind.data.datasets[0].data.length > 0) {
        charts.wind.data.datasets[0].data.forEach(point => {
            if (units.wind === 'kph') {
                point.y = mphToKph(point.y);
            } else {
                point.y = kphToMph(point.y);
            }
        });
        const windAvg = calculateAverage(charts.wind.data.datasets[0].data);
        updateAverageLine(charts.wind, charts.wind.data.datasets[0].data);
        charts.wind.options.scales.y.title = {
            display: true,
            text: units.wind === 'mph' ? 'mph' : 'kph'
        };
        charts.wind.update();
    }

    // Recalculate rain data and average
    if (changedSensor === 'rain' && charts.rain.data.datasets[0].data.length > 0) {
        charts.rain.data.datasets[0].data.forEach(point => {
            if (units.rain === 'mm') {
                point.y = inchesToMm(point.y);
            } else {
                point.y = mmToInches(point.y);
            }
        });
        const rainAvg = calculateAverage(charts.rain.data.datasets[0].data);
        updateAverageLine(charts.rain, charts.rain.data.datasets[0].data);
        // Update 24-hour accumulation line with current units and weatherData
        if (weatherData && weatherData.rainDailyTotal !== undefined) {
            update24HourAccumulationLine(charts.rain, weatherData.rainDailyTotal, units);
        }
        // Update accumulated rain line
        updateAccumulatedRainLine(charts.rain);
        charts.rain.update();
    }

    // Recalculate pressure data and average
    if (changedSensor === 'pressure' && charts.pressure.data.datasets[0].data.length > 0) {
        charts.pressure.data.datasets[0].data.forEach(point => {
            if (units.pressure === 'inHg') {
                point.y = mbToInHg(point.y);
            } else {
                point.y = inHgToMb(point.y);
            }
        });
        const pressureAvg = calculateAverage(charts.pressure.data.datasets[0].data);
        updateAverageLine(charts.pressure, charts.pressure.data.datasets[0].data);
        updateTrendLine(charts.pressure, charts.pressure.data.datasets[0].data);
        charts.pressure.update();
    }

    // Update pressure analysis when units change - AGGRESSIVE DEBUGGING (v3.0)
    if (weatherData) {
        console.log('🔄 UNITS CHANGE - Pressure Update:', weatherData);
        const conditionEl = document.getElementById('pressure-condition');
        const trendEl = document.getElementById('pressure-trend');
        const forecastEl = document.getElementById('pressure-forecast');
        
        if (conditionEl) conditionEl.textContent = weatherData.pressure_condition || '--';
        if (trendEl) trendEl.textContent = weatherData.pressure_trend || '--';
        if (forecastEl) forecastEl.textContent = weatherData.weather_forecast || '--';

        // Update daily rain total when units change
        const dailyRainElement = document.getElementById('daily-rain-total');
        if (dailyRainElement && weatherData.rainDailyTotal !== undefined) {
            let dailyRain = weatherData.rainDailyTotal;
            if (units.rain === 'mm') {
                dailyRain = inchesToMm(dailyRain);
            }
            const rainUnit = units.rain === 'inches' ? 'in' : 'mm';
            dailyRainElement.textContent = dailyRain.toFixed(3) + ' ' + rainUnit;
        }
    }
    
    // Always update chart axis titles to ensure they reflect current units
    // This doesn't affect data conversion, just display labels
    if (charts.wind && charts.wind.options && charts.wind.options.scales) {
        charts.wind.options.scales.y.title = {
            display: true,
            text: units.wind === 'mph' ? 'mph' : 'kph'
        };
        charts.wind.update();
    }
}

// Unit conversion functions
/**
 * Format a temperature for display in the current unit, with 1 decimal and the unit symbol.
 * @param {number} celsius Temperature in °C.
 * @returns {string} E.g. `"21.4°C"` or `"70.5°F"`.
 */
function formatTemperature(celsius) {
    if (units.temperature === 'fahrenheit') {
        return `${(celsius * 9/5 + 32).toFixed(1)}°F`;
    }
    return `${celsius.toFixed(1)}°C`;
}

/**
 * Format a pressure for display: inHg with 3 decimals or mb with 1 decimal.
 * @param {number} mb Pressure in millibars.
 * @returns {string}
 */
function formatPressure(mb) {
    if (units.pressure === 'inHg') {
        return `${(mb * 0.02953).toFixed(3)} inHg`;
    }
    return `${mb.toFixed(1)} mb`;
}

/**
 * Format a wind speed for display, treating the input as metres per second.
 *
 * Converts to mph or km/h when {@link units}.wind is `'mph'` or `'kmh'`; any other
 * value (including `'kph'`, which {@link toggleUnit} sets) shows m/s.
 * @param {number} mps Wind speed in m/s.
 * @returns {string}
 */
function formatWindSpeed(mps) {
    if (units.wind === 'mph') {
        return `${(mps * 2.23694).toFixed(1)} mph`;
    } else if (units.wind === 'kmh') {
        return `${(mps * 3.6).toFixed(1)} km/h`;
    }
    return `${mps.toFixed(1)} m/s`;
}

/**
 * Format a rain amount: inches with 2 decimals or mm with 1 decimal.
 * @param {number} mm Rain amount in millimetres.
 * @returns {string}
 */
function formatRain(mm) {
    if (units.rain === 'inches') {
        return `${(mm * 0.0393701).toFixed(2)} in`;
    }
    return `${mm.toFixed(1)} mm`;
}

/**
 * Format a rain rate: in/hr with 2 decimals or mm/hr with 1 decimal.
 * @param {number} mmPerHour Rain rate in mm/hr.
 * @returns {string}
 */
function formatRainRate(mmPerHour) {
    if (units.rain === 'inches') {
        return `${(mmPerHour * 0.0393701).toFixed(2)} in/hr`;
    }
    return `${mmPerHour.toFixed(1)} mm/hr`;
}

/**
 * Fetch `/api/weather`, store it in {@link weatherData}, and refresh the display and charts.
 *
 * {@link updateDisplay} and {@link updateCharts} are each guarded so a single UI
 * error does not abort the pipeline. On success the header status shows
 * "Connected", the initial-fetch flag is set and readiness is re-checked. On any
 * failure (network, non-2xx, bad JSON) the error is logged and the header shows
 * "Waiting for UDP stream" in UDP mode or "Disconnected" otherwise; nothing is thrown.
 * @async
 * @returns {Promise<void>}
 */
async function fetchWeather() {
    const startTime = performance.now();
    debugLog(logLevels.DEBUG, 'Starting weather API call');
    
    try {
        const response = await fetch('/api/weather');
        const endTime = performance.now();
        const responseTime = endTime - startTime;
        
        debugLog(logLevels.DEBUG, 'Weather API response received', {
            status: response.status,
            statusText: response.statusText,
            responseTime: responseTime.toFixed(2) + 'ms',
            headers: Object.fromEntries(response.headers.entries())
        });
        
        if (response.ok) {
            const rawData = await response.text();
            debugLog(logLevels.DEBUG, 'Raw weather API response', {
                responseLength: rawData.length,
                responsePreview: rawData.substring(0, 200) + (rawData.length > 200 ? '...' : '')
            });
            
            weatherData = JSON.parse(rawData);
            debugLog(logLevels.INFO, 'Weather data successfully parsed', {
                temperature: weatherData.temperature,
                humidity: weatherData.humidity,
                pressure: weatherData.pressure,
                pressure_condition: weatherData.pressure_condition,
                pressure_trend: weatherData.pressure_trend,
                weather_forecast: weatherData.weather_forecast,
                illuminance: weatherData.illuminance,
                uv: weatherData.uv,
                lastUpdate: weatherData.lastUpdate
            });
            
            // Guard updateDisplay to prevent a single UI error from aborting the pipeline
            try {
                updateDisplay();
            } catch (displayErr) {
                console.error('❌ ERROR in updateDisplay:', displayErr);
                debugLog(logLevels.ERROR, 'updateDisplay error', { error: displayErr.message, stack: displayErr.stack });
            }

            // Explicit chart update with error handling
            try {
                console.log('🚀 DEBUG: About to call updateCharts with weatherData:', weatherData);
                updateCharts();
                console.log('🚀 DEBUG: updateCharts completed successfully');
            } catch (error) {
                console.error('❌ ERROR in updateCharts:', error);
                // Log error to server as well
                debugLog(logLevels.ERROR, 'updateCharts error', { error: error.message, stack: error.stack });
            }
            
            // Only update status element if it exists (not present on chart-only popout pages)
            const statusEl = document.getElementById('status');
            if (statusEl) {
                statusEl.textContent = 'Connected to Tempest station';
                statusEl.style.background = 'rgba(40,  167, 69, 0.1)';
            }
            
            console.log('🚀 DEBUG: fetchWeather completed, calling updateCharts');
            debugLog(logLevels.INFO, 'Weather fetch completed successfully', {
                totalTime: (performance.now() - startTime).toFixed(2) + 'ms'
            });
            // mark initial weather fetch completed for readiness gating
            __weatherFetched = true;
            trySetDashboardReady();
        } else {
            throw new Error(`Weather API error: ${response.status} ${response.statusText}`);
        }
    } catch (error) {
        debugLog(logLevels.ERROR, 'Error fetching weather data', {
            error: error.message,
            stack: error.stack
        });
        
        // Check if we're in UDP mode - show appropriate waiting message
        const statusEl = document.getElementById('status');
        if (statusEl) {
            if (currentDataSourceType === 'udp') {
                // UDP mode - show waiting message instead of error
                statusEl.textContent = 'Waiting for UDP stream from Tempest station';
                statusEl.style.background = 'rgba(255, 193, 7, 0.1)'; // Yellow background
            } else if (!statusEl.textContent.includes('Waiting')) {
                // Non-UDP mode - show disconnected
                statusEl.textContent = 'Disconnected from weather station';
                statusEl.style.background = 'rgba(220, 53, 69, 0.1)';
            }
        }
    }
}

/**
 * Load historical data for the popout chart page from `/api/history`.
 *
 * Appends each observation (converted to display units) to the popout's main
 * dataset, then computes the average line, the pressure trend, or for rain the
 * window accumulation and the midnight-aware "today total" step line. Errors are
 * logged, not thrown.
 * @async
 * @returns {Promise<void>}
 */
async function loadHistoricalDataForPopout() {
    if (!charts.popout || !charts.popoutType) {
        debugLog(logLevels.WARN, 'Cannot load historical data - popout chart not initialized');
        return;
    }
    
    try {
        debugLog(logLevels.INFO, 'Loading historical data for popout chart', { type: charts.popoutType });
        const response = await fetch('/api/history');
        if (!response.ok) {
            throw new Error(`History API returned ${response.status}`);
        }
        
        const history = await response.json();
        if (!history || !Array.isArray(history)) {
            debugLog(logLevels.WARN, 'No historical data available');
            return;
        }
        
        debugLog(logLevels.INFO, `Loaded ${history.length} historical data points for popout chart`);
        
    // Process each historical observation and add to chart
    const chartType = charts.popoutType;
    // Main data is the first dataset in popout charts (drawn underneath);
    // average/trend/accumulated lines are appended after it.
    const datasetIndex = 0;
        
        history.forEach(obs => {
            if (!obs || !obs.timestamp) return;
            
            const timestamp = new Date(obs.timestamp * 1000);
            let value = 0;
            
            switch(chartType) {
                case 'temperature':
                    value = obs.air_temperature || 0;
                    if (units.temperature === 'fahrenheit') {
                        value = celsiusToFahrenheit(value);
                    }
                    break;
                case 'humidity':
                    value = obs.relative_humidity || 0;
                    break;
                case 'wind':
                    value = obs.wind_avg || 0;
                    if (units.wind === 'kph') {
                        value = mphToKph(value);
                    }
                    break;
                case 'rain':
                    // Use rainRate (rain intensity in mm/hr) for the intensity chart
                    // Convert to inches/hr if user prefers inches
                    value = obs.rainRate || 0;
                    if (units.rain === 'inches') {
                        value = mmToInches(value); // Convert mm/hr to in/hr
                    }
                    break;
                case 'pressure':
                    value = obs.station_pressure || 0;
                    if (units.pressure === 'inHg') {
                        value = mbToInHg(value);
                    }
                    break;
                case 'light':
                    value = obs.illuminance || 0;
                    break;
                case 'uv':
                    value = obs.uv || 0;
                    break;
            }
            
            charts.popout.data.datasets[datasetIndex].data.push({ x: timestamp, y: value });
        });
        
        // Calculate and populate average line (dataset 1) - skip for light and UV
        const mainData = charts.popout.data.datasets[datasetIndex].data;
        if (mainData.length > 0) {
            const firstX = mainData[0].x;
            const lastX = mainData[mainData.length - 1].x;
            
            // Calculate average for charts that have an average line
            if (chartType !== 'light' && chartType !== 'uv') {
                let sum = 0;
                let count = 0;
                for (let i = 0; i < mainData.length; i++) {
                    if (mainData[i] && typeof mainData[i].y === 'number') {
                        sum += mainData[i].y;
                        count++;
                    }
                }
                const avg = count > 0 ? sum / count : 0;
                // average is dataset index 1 when present
                if (charts.popout.data.datasets[1]) {
                    charts.popout.data.datasets[1].data = [
                    { x: firstX, y: avg },
                    { x: lastX, y: avg }
                    ];
                }
            }
            
            // Calculate trend line for pressure (dataset[1])
            // pressure: main(0), average(1), trend(2)
            if (chartType === 'pressure' && charts.popout.data.datasets[2]) {
                const trendData = calculateTrendLine(mainData);
                charts.popout.data.datasets[2].data = trendData;
            }
            
            // Calculate accumulated line for rain (dataset[1]) and today total (dataset[2])
            if (chartType === 'rain') {
                // Build midnight-aware accumulated line and per-point today-total step.
                if (mainData.length > 0) {
                    /**
                     * Local date key (YYYY-MM-DD) for grouping rain by calendar day.
                     * @param {Date|number} ts Timestamp.
                     * @returns {string}
                     */
                    const localDateKey = ts => {
                        const d = new Date(ts);
                        const y = d.getFullYear();
                        const m = String(d.getMonth() + 1).padStart(2, '0');
                        const day = String(d.getDate()).padStart(2, '0');
                        return `${y}-${m}-${day}`;
                    };

                    const runningByDay = {};
                    const cumulativeSinceMidnight = new Array(mainData.length).fill(0);
                    let cumulativeWindow = 0;
                    const accumulatedData = [];

                    for (let i = 0; i < mainData.length; i++) {
                        if (i === 0) {
                            cumulativeSinceMidnight[i] = 0;
                            accumulatedData.push({ x: mainData[i].x, y: units.rain === 'inches' ? mmToInches(0) : 0 });
                            continue;
                        }

                        const t0 = mainData[i - 1].x;
                        const t1 = mainData[i].x;
                        const intensity = mainData[i].y || 0; // may be mm/hr or inches/hr depending on units
                        const intensityMm = (units.rain === 'inches') ? inchesToMm(intensity) : intensity;
                        let segStart = t0;
                        while (segStart < t1) {
                            const segStartDate = new Date(segStart);
                            const nextMidnight = new Date(segStartDate);
                            nextMidnight.setHours(24, 0, 0, 0);
                            const segEnd = Math.min(t1, nextMidnight.getTime());
                            const segHours = (segEnd - segStart) / (1000 * 60 * 60);
                            const delta = intensity * segHours;
                            const dayKey = localDateKey(segStart);
                            runningByDay[dayKey] = (runningByDay[dayKey] || 0) + delta;
                            segStart = segEnd;
                        }

                        const thisDayKey = localDateKey(mainData[i].x);
                        cumulativeSinceMidnight[i] = runningByDay[thisDayKey] || 0;
                        cumulativeWindow += intensityMm * ((t1 - t0) / (1000 * 60 * 60));
                        const dispAccum = (units.rain === 'inches') ? mmToInches(cumulativeWindow) : cumulativeWindow;
                        accumulatedData.push({ x: mainData[i].x, y: dispAccum });
                    }

                    // Assign to accumulation dataset (find by label)
                    const accumDs = charts.popout.data.datasets.find(ds => ds.label && ds.label.toLowerCase().includes('accumul'));
                    if (accumDs) accumDs.data = accumulatedData;

                    // Build today-total data (step) and assign
                    const todayKey = localDateKey(Date.now());
                    const todayDs = charts.popout.data.datasets.find(ds => ds.label && ds.label.toLowerCase().includes('today'));
                    if (todayDs) {
                        const todayData = [];
                        for (let i = 0; i < mainData.length; i++) {
                            const pk = localDateKey(mainData[i].x);
                            let yVal = 0;
                            if (pk === todayKey) {
                                yVal = cumulativeSinceMidnight[i] || 0;
                            } else {
                                yVal = runningByDay[pk] || 0;
                            }
                            if (units.rain === 'inches') yVal = mmToInches(yVal);
                            todayData.push({ x: mainData[i].x, y: yVal });
                        }
                        todayDs.data = todayData;
                        todayDs.stepped = true;
                        debugLog(logLevels.INFO, 'Rain accumulated line calculated for popout', {
                            dataPoints: accumulatedData.length,
                            mainDataPoints: mainData.length,
                            finalTotal: (units.rain === 'inches' ? mmToInches(cumulativeWindow) : cumulativeWindow).toFixed(3),
                            unit: units.rain
                        });
                    }
                }
            }
        }
        
        // Update the chart
        try {
            charts.popout.update();
            const debugInfo = {
                chartType: chartType,
                dataPoints: mainData.length,
                // avgPoints lives at index 1 when present
                avgPoints: charts.popout.data.datasets[1] ? charts.popout.data.datasets[1].data.length : 0,
                // trend (pressure) lives at index 2 when present
                trendPoints: (chartType === 'pressure' && charts.popout.data.datasets[2]) ? charts.popout.data.datasets[2].data.length : 0,
                totalDatasets: charts.popout.data.datasets.length
            };
            if (chartType === 'rain') {
                debugInfo.accumulatedPoints = charts.popout.data.datasets[2] ? charts.popout.data.datasets[2].data.length : 0;
                debugInfo.todayTotalPoints = charts.popout.data.datasets[3] ? charts.popout.data.datasets[3].data.length : 0;
                debugInfo.dataset0_length = charts.popout.data.datasets[0] ? charts.popout.data.datasets[0].data.length : 0;
                debugInfo.dataset1_length = charts.popout.data.datasets[1] ? charts.popout.data.datasets[1].data.length : 0;
                debugInfo.dataset2_length = charts.popout.data.datasets[2] ? charts.popout.data.datasets[2].data.length : 0;
                debugInfo.dataset3_length = charts.popout.data.datasets[3] ? charts.popout.data.datasets[3].data.length : 0;
            }
            debugLog(logLevels.INFO, 'Popout chart updated with historical data', debugInfo);
        } catch (e) {
            debugLog(logLevels.ERROR, 'Failed to update popout chart', e);
        }
    } catch (error) {
        debugLog(logLevels.ERROR, 'Failed to load historical data for popout', {
            error: error.message
        });
    }
}

/**
 * Fetch `/api/status` and render the station, forecast and HomeKit sections.
 *
 * Exposes the raw JSON as `window.__lastStatusRaw` for headless tests, records the
 * data source type in {@link currentDataSourceType}, and marks the initial status
 * fetch complete for readiness gating. Errors are logged, not thrown.
 * @async
 * @returns {Promise<void>}
 */
async function fetchStatus() {
    const startTime = performance.now();
    const responseTime = (performance.now() - startTime).toFixed(2);
    
    try {
        const response = await fetch('/api/status');
        if (response.ok) {
            const status = await response.json();
            // expose raw status JSON string for headless tests to inspect exact payload
            try {
                window.__lastStatusRaw = JSON.stringify(status);
            } catch(e) {
                // ignore
            }
            debugLog(logLevels.DEBUG, 'Status API response received', {
                responseTime: responseTime + 'ms',
                connected: status.connected,
                stationName: status.stationName,
                uptime: status.uptime,
                homekitStatus: status.homekit
            });
            
            // Store data source type globally for better error messaging
            if (status.dataSource) {
                currentDataSourceType = status.dataSource.type;
            }
            
            updateStatusDisplay(status);
            updateForecastDisplay(status);
            // mark initial status fetch completed for readiness gating
            __statusFetched = true;
            trySetDashboardReady();
        } else {
            throw new Error(`Status API error: ${response.status}`);
        }
    } catch (error) {
        debugLog(logLevels.ERROR, 'Error fetching status', {
            error: error.message,
            responseTime: (performance.now() - startTime).toFixed(2) + 'ms'
        });
    }
}

/**
 * Render a `/api/status` payload into the Tempest station and HomeKit cards.
 *
 * Stores it in {@link statusData}. Shows history-loading progress, the connection
 * state (generated, waiting for UDP, connected/disconnected), the station name (a
 * regenerate link in generated-weather mode), the station URL or UDP packet
 * count/IP, elevation, last update, uptime, battery and observation counts. Also
 * feeds `status.dataHistory` to {@link populateChartsWithHistoricalData} and calls
 * {@link updateDetailedStationStatus} and {@link updateHomekitStatus}. Skipped on
 * chart-only popout pages.
 *
 * @param {Object} status Parsed `/api/status` response.
 * @returns {void}
 */
function updateStatusDisplay(status) {
    debugLog(logLevels.DEBUG, 'Updating status display', status);
    debugLog(logLevels.DEBUG, '🔍 STATUS DEBUG - Full status object:', JSON.stringify(status, null, 2));
    
    // Skip status display updates on chart-only popout pages
    if (!document.getElementById('tempest-status')) {
        debugLog(logLevels.DEBUG, 'Skipping status display update on chart-only page');
        return;
    }
    
    // Store status data globally for unit conversions
    statusData = status;
    
    // Update Tempest status
    const tempestStatus = document.getElementById('tempest-status');
    const tempestStation = document.getElementById('tempest-station');
    const tempestStationURL = document.getElementById('tempest-station-url');
    const tempestElevation = document.getElementById('tempest-elevation');
    const tempestLastUpdate = document.getElementById('tempest-last-update');
    const tempestUptime = document.getElementById('tempest-uptime');
    const tempestDataCount = document.getElementById('tempest-data-count');
    const tempestHistoricalRow = document.getElementById('tempest-historical-row');
    const tempestHistoricalCount = document.getElementById('tempest-historical-count');

    // Handle historical data loading progress first
    if (status.historyLoadingProgress && status.historyLoadingProgress.isLoading) {
        // Update status to show we're reading historical data
        if (tempestStatus) {
            tempestStatus.textContent = '⏳ Reading Historical Observations';
            tempestStatus.style.color = '#ffc107'; // Yellow color for loading
        }
    } else {
        // Not loading - show normal connection status
        if (tempestStatus) {
            if (status.generatedWeather && status.generatedWeather.enabled) {
                tempestStatus.textContent = '🔧 Generated';
                tempestStatus.style.color = '#17a2b8'; // Info blue for generated
            } else if (!status.connected && status.dataSource && status.dataSource.type === 'udp') {
                // UDP mode but not connected yet - show waiting message
                tempestStatus.textContent = '⏳ Waiting for UDP Stream';
                tempestStatus.style.color = '#ffc107'; // Yellow/warning color
                // Also update the main header status
                const mainStatus = document.getElementById('status');
                if (mainStatus) {
                    mainStatus.textContent = 'Waiting for UDP stream from Tempest station';
                    mainStatus.style.background = 'rgba(255, 193, 7, 0.1)'; // Yellow background
                }
            } else {
                tempestStatus.textContent = status.connected ? '✅ Connected' : '❌ Disconnected';
                tempestStatus.style.color = status.connected ? '#28a745' : '#dc3545';
            }
        }
    }
    
    // Update station name or generated weather location
    if (tempestStation) {
        if (status.generatedWeather && status.generatedWeather.enabled) {
            tempestStation.innerHTML = `<span style="cursor: pointer; color: #007bff; text-decoration: underline;" onclick="regenerateWeather()" title="Click to regenerate with new location/season">${status.generatedWeather.location} (${status.generatedWeather.season})</span>`;
        } else {
            tempestStation.textContent = status.stationName || '--';
        }
    }
    
    // Update station URL or UDP info
    const tempestStationURLLabel = document.getElementById('tempest-station-url-label');
    if (tempestStationURL && tempestStationURLLabel) {
        // Check if we're in UDP mode
        if (status.dataSource && status.dataSource.type === 'udp') {
            // UDP mode: Change label and show packet count + IP
            tempestStationURLLabel.textContent = 'Station UDP:';
            const packets = status.dataSource.packetCount || 0;
            const ip = status.dataSource.stationIP || '';
            if (ip) {
                tempestStationURL.textContent = `${packets} packets, ${ip}`;
            } else {
                tempestStationURL.textContent = `${packets} packets`;
            }
        } else if (status.stationURL) {
            // Non-UDP mode: Show station URL as before
            tempestStationURLLabel.textContent = 'Station URL:';
            // Make the URL clickable and truncate if too long
            // Truncate display label to 15 characters for compact card layout, show full URL on hover
            const maxLabelLen = 15;
            let displayURL = status.stationURL;
            if (displayURL.length > maxLabelLen) {
                displayURL = displayURL.substring(0, maxLabelLen - 1) + '…';
            }
            // Provide full URL in title and aria-label for hover and accessibility
            tempestStationURL.innerHTML = `<a href="${status.stationURL}" target="_blank" style="color: #007bff; text-decoration: none;" title="${status.stationURL}" aria-label="${status.stationURL}">${displayURL}</a>`;
        } else {
            // No URL available
            tempestStationURLLabel.textContent = 'Station URL:';
            tempestStationURL.textContent = '--';
        }
    }
    
    // Update elevation display with unit conversion
    updateElevationDisplay();
    
    if (tempestLastUpdate) tempestLastUpdate.textContent = status.lastUpdate ? new Date(status.lastUpdate).toLocaleString('en-US', {
        year: 'numeric',
        month: '2-digit', 
        day: '2-digit',
        hour: '2-digit',
        minute: '2-digit',
        hour12: false
    }) : '--';
    if (tempestUptime) tempestUptime.textContent = status.uptime || '--';
    
    // Update battery level and device uptime from station status
    const tempestBattery = document.getElementById('tempest-battery');
    
    // Use station status data if available, otherwise fall back to weather data
    if (status.stationStatus && status.stationStatus.batteryVoltage) {
        tempestBattery.textContent = status.stationStatus.batteryVoltage;
        debugLog(logLevels.DEBUG, '🔋 Battery updated from station status:', status.stationStatus.batteryVoltage);
        
        // Update uptime with more accurate device uptime
        if (tempestUptime && status.stationStatus.deviceUptime) {
            tempestUptime.textContent = status.stationStatus.deviceUptime;
            debugLog(logLevels.DEBUG, '⏱️ Device uptime updated from station status:', status.stationStatus.deviceUptime);
        }
    } else if (tempestBattery && status.dataHistory && status.dataHistory.length > 0) {
        // Fallback to weather data
        const latestData = status.dataHistory[status.dataHistory.length - 1];
        if (latestData.battery !== undefined && latestData.battery !== 0) {
            tempestBattery.textContent = `${latestData.battery.toFixed(1)}V`;
            debugLog(logLevels.DEBUG, '🔋 Battery set from weather data:', `${latestData.battery.toFixed(1)}V`);
        } else {
            tempestBattery.textContent = 'N/A';
            debugLog(logLevels.DEBUG, '🔋 Battery data not available');
        }
    } else if (tempestBattery) {
        tempestBattery.textContent = '--';
        debugLog(logLevels.DEBUG, '🔋 No battery data available');
    }
    
    if (tempestDataCount) tempestDataCount.textContent = status.observationCount > 0 ? `${status.observationCount}/${status.maxHistorySize}` : '0';
    
    // Handle historical data loading progress
    if (status.historyLoadingProgress && status.historyLoadingProgress.isLoading) {
        // Show and update historical row with progress
        if (tempestHistoricalRow && tempestHistoricalCount) {
            tempestHistoricalRow.style.display = '';
            const progressText = `${status.historyLoadingProgress.currentStep}/${status.historyLoadingProgress.totalSteps}`;
            tempestHistoricalCount.textContent = progressText;
        }
    } else {
        // Show/hide historical data row and update count (normal state)
        if (tempestHistoricalRow && tempestHistoricalCount) {
            if (status.observationCount > 0) {
                tempestHistoricalRow.style.display = '';
                tempestHistoricalCount.textContent = `${status.observationCount}/${status.maxHistorySize} data points`;
            } else {
                tempestHistoricalRow.style.display = 'none';
            }
        }
    }

    // Populate charts with historical data if available
    if (status.dataHistory && status.dataHistory.length > 0) {
        populateChartsWithHistoricalData(status.dataHistory);
    }

    // Update detailed station status from stationStatus data
    updateDetailedStationStatus(status);

    // Update HomeKit status
    const homekitStatus = document.getElementById('homekit-status');
    const homekitAccessories = document.getElementById('homekit-accessories');
    const homekitBridge = document.getElementById('homekit-bridge');
    const homekitPin = document.getElementById('homekit-pin');

    const hk = status.homekit || {};
    if (homekitStatus) {
        if (hk.bridge) {
            homekitStatus.textContent = '✅ Active';
            homekitStatus.style.color = '#28a745';
        } else if (hk.status && hk.status.includes('Disabled')) {
            homekitStatus.textContent = '❌ Disabled';
            homekitStatus.style.color = '#dc3545';
        } else {
            homekitStatus.textContent = '⚠️ Inactive';
            homekitStatus.style.color = '#ffc107';
        }
    }
    if (homekitAccessories) homekitAccessories.textContent = hk.accessories || '--';
    if (homekitBridge) homekitBridge.textContent = hk.name || '--';
    if (homekitPin) homekitPin.textContent = hk.pin || '--';

    // Update detailed HomeKit connection info, technical details, and accessories
    updateHomekitStatus(status);
    
    debugLog(logLevels.DEBUG, 'Status display update completed', {
        tempestConnected: status.connected,
        homekitActive: hk.bridge,
        accessoryCount: hk.accessories
    });
}

/**
 * Updates the data source display based on the unified data source status.
 *
 * Shows all active data sources as a comma-separated list: the observation
 * source (UDP Stream, Generated, Custom URL, WeatherFlow API), API forecast/history
 * use, and web-status scraping (or its API fallback). Falls back to "Generated"
 * or "API" when nothing is detected.
 *
 * @param {Object} status Parsed `/api/status` response.
 * @param {?HTMLElement} dataSourceElement Target element; nothing happens if it is null.
 * @returns {void}
 */
function updateDataSourceDisplay(status, dataSourceElement) {
    if (!dataSourceElement) return;
    
    // Debug: Log the entire status object structure
    debugLog(logLevels.DEBUG, '📡 updateDataSourceDisplay called with:', {
        hasDataSource: !!status.dataSource,
        dataSourceType: status.dataSource?.type,
        packetCount: status.dataSource?.packetCount,
        stationIP: status.dataSource?.stationIP,
        hasStationName: !!status.stationName,
        hasStationURL: !!status.stationURL,
        hasForecast: !!status.forecast,
        hasStationStatus: !!status.stationStatus,
        statusDataSource: status.stationStatus?.dataSource
    });
    
    const sources = [];
    
    // Primary observation source (what's feeding real-time data)
    if (status.dataSource) {
        const dsType = status.dataSource.type;
        if (dsType === 'udp') {
            // UDP Stream - details moved to Station UDP row
            sources.push('UDP Stream');
        } else if (dsType === 'generated') {
            sources.push('Generated');
        } else if (dsType === 'custom-url') {
            sources.push('Custom URL');
        } else if (dsType === 'api') {
            sources.push('WeatherFlow API');
        }
    }
    
    // API usage for forecast/history (when using UDP for observations)
    if (status.dataSource && status.dataSource.type === 'udp') {
        // Check if forecast data is actually available (indicates API is accessible)
        if (status.forecast && status.forecast.forecast) {
            // Show history only if historical data was actually loaded
            if (status.historicalDataLoaded) {
                sources.push('API (forecast, history)');
            } else {
                sources.push('API (forecast)');
            }
        }
    }
    
    // Forecast source (for non-UDP modes that have separate forecast)
    if (status.forecast && status.dataSource) {
        // Only add if not already API or UDP (which already mentions API)
        if (status.dataSource.type !== 'api' && status.dataSource.type !== 'udp') {
            sources.push('API Forecast');
        }
    }
    
    // Web-Status scraping (if available)
    if (status.stationStatus) {
        const statusSource = status.stationStatus.dataSource || '';
        if (statusSource.includes('web status page') || statusSource === 'web-scrape') {
            sources.push('Web-Status');
        } else if (status.stationStatus.scrapingEnabled && statusSource.includes('api')) {
            // Scraping is enabled but using API fallback
            sources.push('Web-Status (API fallback)');
        }
    }
    
    // If no sources detected, show fallback
    if (sources.length === 0) {
        if (status.generatedWeather && status.generatedWeather.enabled) {
            sources.push('Generated');
        } else {
            sources.push('API');
        }
    }
    
    // Join sources with commas
    dataSourceElement.textContent = sources.join(', ');
    
    debugLog(logLevels.DEBUG, '📡 Data Sources:', sources.join(', '));
}

/**
 * Fill the detailed device, hub and data-source rows of the Tempest card.
 *
 * When `status.stationStatus.batteryVoltage` is present, copies the device and hub
 * fields (uptime, network, signal with bars, serial, firmware, sensor and battery
 * status) and updates the battery indicator; otherwise every field shows `--`.
 * @param {Object} status Parsed `/api/status` response.
 * @returns {void}
 */
function updateDetailedStationStatus(status) {
    debugLog(logLevels.DEBUG, 'updateDetailedStationStatus called', status);
    
    const stationStatus = status.stationStatus;
    
    // Device Status Fields
    const deviceUptime = document.getElementById('tempest-device-uptime');
    const deviceNetwork = document.getElementById('tempest-device-network');
    const deviceSignal = document.getElementById('tempest-device-signal');
    const deviceLastObs = document.getElementById('tempest-device-last-obs');
    const deviceSerial = document.getElementById('tempest-device-serial');
    const deviceFirmware = document.getElementById('tempest-device-firmware');
    const sensorStatus = document.getElementById('tempest-sensor-status');
    const batteryStatus = document.getElementById('tempest-battery-status');

    // Hub Status Fields
    const hubUptime = document.getElementById('tempest-hub-uptime');
    const hubNetwork = document.getElementById('tempest-hub-network');
    const hubWifi = document.getElementById('tempest-hub-wifi');
    const hubLastStatus = document.getElementById('tempest-hub-last-status');
    const hubSerial = document.getElementById('tempest-hub-serial');
    const hubFirmware = document.getElementById('tempest-hub-firmware');

    // Data Source Field
    const dataSource = document.getElementById('tempest-data-source');

    // Debug: Check which fields are missing
    console.log('🔍 Field Availability Check:', {
        'deviceLastObs exists': !!deviceLastObs,
        'deviceSerial exists': !!deviceSerial,
        'deviceFirmware exists': !!deviceFirmware,
        'hubLastStatus exists': !!hubLastStatus,
        'hubSerial exists': !!hubSerial,
        'hubFirmware exists': !!hubFirmware
    });

    // Update Data Source field from unified data source status
    updateDataSourceDisplay(status, dataSource);

    if (stationStatus && stationStatus.batteryVoltage) {
        
        // Update Device Status fields from actual station status
        if (deviceUptime) deviceUptime.textContent = stationStatus.deviceUptime || '--';
        if (deviceNetwork) deviceNetwork.textContent = stationStatus.deviceNetworkStatus || '--';
        if (deviceSignal) {
            deviceSignal.textContent = stationStatus.deviceSignal || '--';
            // Update signal bars
            createSignalBars(stationStatus.deviceSignal, 'tempest-device-signal-bars');
        }
        if (deviceLastObs) deviceLastObs.textContent = stationStatus.deviceLastObs || '--';
        if (deviceSerial) deviceSerial.textContent = stationStatus.deviceSerialNumber || '--';
        if (deviceFirmware) deviceFirmware.textContent = stationStatus.deviceFirmware || '--';
        if (sensorStatus) sensorStatus.textContent = stationStatus.sensorStatus || '--';
        if (batteryStatus) batteryStatus.textContent = stationStatus.batteryStatus || '--';
        
        // Update battery indicator with color coding
        updateBatteryIndicator(stationStatus.batteryVoltage);

        // Update Hub Status fields from actual station status
        if (hubUptime) hubUptime.textContent = stationStatus.hubUptime || '--';
        if (hubNetwork) hubNetwork.textContent = stationStatus.hubNetworkStatus || '--';
        if (hubWifi) {
            hubWifi.textContent = stationStatus.hubWiFiSignal || '--';
            // Update signal bars for hub
            createSignalBars(stationStatus.hubWiFiSignal, 'tempest-hub-signal-bars');
        }
        if (hubLastStatus) hubLastStatus.textContent = stationStatus.hubLastStatus || '--';
        if (hubSerial) hubSerial.textContent = stationStatus.hubSerialNumber || '--';
        if (hubFirmware) hubFirmware.textContent = stationStatus.hubFirmware || '--';

        debugLog(logLevels.DEBUG, 'Detailed station status updated from stationStatus data');
    } else {
        // Use "--" for all fields when station status is not available
        const allStatusFields = [
            deviceUptime, deviceNetwork, deviceSignal, deviceLastObs, deviceSerial, deviceFirmware,
            sensorStatus, batteryStatus, hubUptime, hubNetwork, hubWifi, hubLastStatus, hubSerial, hubFirmware
        ];
        
        allStatusFields.forEach((field, index) => {
            if (field) {
                field.textContent = '--';
            }
        });
        
        debugLog(logLevels.DEBUG, 'Station status data unavailable, using "--" for all detailed status fields');
    }
}

/**
 * Show or hide the forecast card and render `status.forecast`.
 *
 * Caches the forecast in {@link forecastData} for later unit changes, then renders
 * the current conditions and the daily forecast when present.
 * @param {Object} status Parsed `/api/status` response.
 * @returns {void}
 */
function updateForecastDisplay(status) {
    debugLog(logLevels.DEBUG, 'Updating forecast display', status.forecast);

    const forecastCard = document.getElementById('forecast-card');
    
    if (!status.forecast) {
        debugLog(logLevels.DEBUG, 'No forecast data available');
        // Hide the forecast card when no data
        if (forecastCard) {
            forecastCard.style.display = 'none';
        }
        return;
    }

    // Show the forecast card when data is available
    if (forecastCard) {
        forecastCard.style.display = 'block';
    }

    const forecast = status.forecast;
    console.log('🌤️ RAW FORECAST DATA FROM API:', JSON.stringify(forecast, null, 2));
    debugLog(logLevels.DEBUG, 'Forecast data structure:', forecast);

    // Store forecast data globally for unit conversions
    forecastData = forecast;

    // Update current conditions
    if (forecast.current_conditions) {
        debugLog(logLevels.DEBUG, 'Updating current conditions');
        updateCurrentConditions(forecast.current_conditions);
    } else {
        debugLog(logLevels.DEBUG, 'No current conditions data');
    }

    // Update daily forecast
    if (forecast.forecast && forecast.forecast.daily) {
        debugLog(logLevels.DEBUG, 'Updating daily forecast');
        updateDailyForecast(forecast.forecast.daily);
    } else {
        debugLog(logLevels.DEBUG, 'No daily forecast data');
    }
}

/**
 * Refresh the forecast display with the current units (called when units are
 * toggled), using the cached {@link forecastData}. No-op before the first forecast.
 * @returns {void}
 * @throws {TypeError} If the cached forecast lacks `current_conditions` or `forecast.daily`.
 */
function refreshForecastDisplay() {
    // Refresh forecast display with current units (called when units are toggled)
    if (!forecastData) {
        debugLog(logLevels.DEBUG, 'No cached forecast data available for refresh');
        return;
    }
    
    debugLog(logLevels.DEBUG, 'Refreshing forecast display with current units');
    
    // Update current conditions with current units
    updateCurrentConditions(forecastData.current_conditions);
    
    // Update daily forecast with current units  
    updateDailyForecast(forecastData.forecast.daily);
}

/**
 * Render the forecast's current conditions block.
 *
 * Temperatures (°C from the API) and sea-level pressure (mb) are converted to the
 * display units; wind is always labelled mph.
 * @param {Object} current `forecast.current_conditions` from the WeatherFlow forecast.
 * @returns {void}
 */
function updateCurrentConditions(current) {
    debugLog(logLevels.DEBUG, 'Updating current conditions with data:', current);
    
    const elements = {
        icon: document.getElementById('forecast-current-icon'),
        temp: document.getElementById('forecast-current-temp'),
        feelsLike: document.getElementById('forecast-current-feels-like'),
        conditions: document.getElementById('forecast-current-conditions'),
        humidity: document.getElementById('forecast-current-humidity'),
        wind: document.getElementById('forecast-current-wind'),
        pressure: document.getElementById('forecast-current-pressure'),
        precip: document.getElementById('forecast-current-precip')
    };

    // Convert temperatures based on current unit setting
    let currentTemp = current.air_temperature;
    let feelsLikeTemp = current.feels_like;
    let tempUnit = '°C';
    
    debugLog(logLevels.DEBUG, 'Raw temperature values from API:', {
        air_temperature: current.air_temperature,
        feels_like: current.feels_like,
        relative_humidity: current.relative_humidity
    });
    
    if (units.temperature === 'fahrenheit') {
        currentTemp = celsiusToFahrenheit(currentTemp);
        feelsLikeTemp = celsiusToFahrenheit(feelsLikeTemp);
        tempUnit = '°F';
    }
    
    // Convert pressure based on current unit setting
    let pressure = current.sea_level_pressure;
    let pressureUnit = 'mb';
    
    if (units.pressure === 'inHg') {
        pressure = mbToInHg(pressure);
        pressureUnit = 'inHg';
    }

    if (elements.icon) elements.icon.textContent = getWeatherIcon(current.icon);
    if (elements.temp) elements.temp.textContent = `${Math.round(currentTemp)}${tempUnit}`;
    if (elements.feelsLike) elements.feelsLike.textContent = `${Math.round(feelsLikeTemp)}${tempUnit}`;
    if (elements.conditions) elements.conditions.textContent = current.conditions;
    if (elements.humidity) elements.humidity.textContent = `${current.relative_humidity}%`;
    if (elements.wind) elements.wind.textContent = `${Math.round(current.wind_avg)} mph`;
    if (elements.pressure) elements.pressure.textContent = `${Math.round(pressure)} ${pressureUnit}`;
    if (elements.precip) elements.precip.textContent = `${current.precip_probability}%`;
}

/**
 * Render up to 5 daily forecast tiles into `#forecast-daily-container`.
 *
 * Day 0 is labelled "Today". If a day's timestamp is within an hour of day 0's,
 * the date is computed from today instead. Missing high/low temperatures are
 * estimated as `air_temperature` ± 3 °C.
 * @param {Array<Object>} dailyForecast `forecast.forecast.daily` entries.
 * @returns {void}
 */
function updateDailyForecast(dailyForecast) {
    const container = document.getElementById('forecast-daily-container');
    if (!container || !dailyForecast) return;

    // Clear existing forecast items
    container.innerHTML = '';

    // Show first 5 days
    const daysToShow = Math.min(5, dailyForecast.length);
    
    for (let i = 0; i < daysToShow; i++) {
        const day = dailyForecast[i];
        
        debugLog(logLevels.DEBUG, `Daily forecast day ${i} raw data:`, {
            air_temperature: day.air_temperature,
            air_temp_high: day.air_temp_high,
            air_temp_low: day.air_temp_low,
            relative_humidity: day.relative_humidity,
            time: day.time,
            conditions: day.conditions
        });
        
        const forecastDay = document.createElement('div');
        forecastDay.className = 'forecast-day';
        
        // Calculate the date for this forecast day
        // If the API provides correct timestamps for each day, use them
        // Otherwise, fall back to calculating from today's date
        let date;
        if (i === 0) {
            // Today - use current date
            date = new Date();
        } else {
            // Try using the API timestamp first
            date = new Date(day.time * 1000);
            // Check if this timestamp seems wrong (same as first day or invalid)
            if (i > 0 && dailyForecast[0] && Math.abs(day.time - dailyForecast[0].time) < 3600) {
                // Timestamps are too similar (less than 1 hour difference), calculate manually
                date = new Date();
                date.setDate(date.getDate() + i);
            }
        }
        
        const dayName = i === 0 ? 'Today' : date.toLocaleDateString('en-US', { weekday: 'short' });
        
        // Convert temperature based on current unit setting
        // Use high/low temperatures if available, otherwise use air_temperature as high and calculate reasonable low
        let highTemp, lowTemp;
        
        if (day.air_temp_high && day.air_temp_low) {
            // Use actual high/low values from API
            highTemp = day.air_temp_high;
            lowTemp = day.air_temp_low;
        } else {
            // Fallback: use air_temperature as average and estimate high/low
            // This is a reasonable estimate based on typical daily temperature ranges
            const avgTemp = day.air_temperature;
            highTemp = avgTemp + 3; // Add 3°C for high
            lowTemp = avgTemp - 3;  // Subtract 3°C for low
        }
        
        debugLog(logLevels.DEBUG, `Day ${i} temperature conversion:`, {
            before_conversion: { high: highTemp, low: lowTemp, unit: 'C' },
            units_setting: units.temperature
        });
        
        let tempUnit = '°C';
        
        if (units.temperature === 'fahrenheit') {
            highTemp = celsiusToFahrenheit(highTemp);
            lowTemp = celsiusToFahrenheit(lowTemp);
            tempUnit = '°F';
        }
        
        debugLog(logLevels.DEBUG, `Day ${i} after conversion:`, {
            after_conversion: { high: Math.round(highTemp), low: Math.round(lowTemp), unit: tempUnit }
        });
        
        forecastDay.innerHTML = `
            <div class="forecast-day-name">${dayName}</div>
            <div class="forecast-day-icon">${getWeatherIcon(day.icon)}</div>
            <div class="forecast-day-conditions">${day.conditions}</div>
            <div class="forecast-day-temps">
                <span class="forecast-day-high">${Math.round(highTemp)}${tempUnit}</span>
                <span class="forecast-day-low">/${Math.round(lowTemp)}${tempUnit}</span>
            </div>
            <div class="forecast-day-precip">${day.precip_probability}%</div>
        `;
        
        container.appendChild(forecastDay);
    }
}

/**
 * Map a WeatherFlow forecast icon code to an emoji.
 * @param {string} iconCode E.g. `'clear-day'`, `'rain'`.
 * @returns {string} The emoji; `'🌤️'` for unknown codes.
 */
function getWeatherIcon(iconCode) {
    const iconMap = {
        'clear-day': '☀️',
        'clear-night': '🌙',
        'partly-cloudy-day': '⛅',
        'partly-cloudy-night': '☁️',
        'cloudy': '☁️',
        'rain': '🌧️',
        'snow': '❄️',
        'sleet': '🌨️',
        'wind': '💨',
        'fog': '🌫️',
        'thunderstorm': '⛈️'
    };
    
    return iconMap[iconCode] || '🌤️';
}

/**
 * Replace the dashboard chart series with the server's observation history.
 *
 * If the charts are not ready yet, the history is parked in
 * `window.__pendingHistoricalData` for the chart initializer to replay. Applies
 * `statusData.chartHistoryHours` as a cut-off. Only repopulates when the charts
 * are empty or the history has more than 5 extra points with real timestamps, so
 * each status poll does not wipe live data. Accepts both the weather API field
 * names (`temperature`, `lastUpdate`, ...) and the history API names
 * (`air_temperature`, `timestamp` in seconds, ...); points without either
 * timestamp are spaced 10 s apart ending now. Afterwards validates, sorts and
 * recomputes average, trend and accumulation lines and redraws.
 *
 * @param {Array<Object>} dataHistory Observations, oldest first.
 * @returns {void}
 */
function populateChartsWithHistoricalData(dataHistory) {
    debugLog(logLevels.DEBUG, 'Populating charts with historical data', {
        dataPoints: dataHistory.length
    });

    // Check if charts are initialized - if not, defer population
    if (!charts.temperature || !charts.rain || !charts.temperature.data || !charts.rain.data) {
        debugLog(logLevels.WARN, 'Charts not yet initialized - deferring historical data population', {
            hasTemperatureChart: !!charts.temperature,
            hasRainChart: !!charts.rain,
            hasTempData: charts.temperature ? !!charts.temperature.data : false,
            hasRainData: charts.rain ? !!charts.rain.data : false
        });
        // Store the data for later population once charts are ready
        if (!window.__pendingHistoricalData) {
            window.__pendingHistoricalData = dataHistory;
        }
        return;
    }

    // Apply chart history filtering if configured
    let filteredHistory = dataHistory;
    if (statusData && statusData.chartHistoryHours > 0) {
        const hoursMs = statusData.chartHistoryHours * 60 * 60 * 1000;
        const cutoffTime = Date.now() - hoursMs;
        filteredHistory = dataHistory.filter(obs => {
            if (!obs.lastUpdate) return true; // Keep data without timestamps (generated weather)
            const obsTime = new Date(obs.lastUpdate).getTime();
            return obsTime >= cutoffTime;
        });
        debugLog(logLevels.INFO, `Chart history filter applied: ${statusData.chartHistoryHours} hours`, {
            originalPoints: dataHistory.length,
            filteredPoints: filteredHistory.length,
            cutoffTime: new Date(cutoffTime).toISOString()
        });
        dataHistory = filteredHistory; // Replace with filtered data
    }

    // Check if we have any historical data with actual timestamps
    const hasActualTimestamps = dataHistory.some(obs => obs.lastUpdate);
    const currentDataLength = charts.temperature.data.datasets[0].data.length;
    
    // Only populate historical data if:
    // 1. Charts are empty (initial load)
    // 2. We have MORE historical data points than current chart data (new historical data loaded)
    // This prevents clearing charts on every status update when dataHistory has fewer points
    const shouldPopulate = currentDataLength === 0 || (hasActualTimestamps && dataHistory.length > currentDataLength + 5);
    
    if (shouldPopulate) {
        debugLog(logLevels.INFO, 'Processing historical data', {
            reason: currentDataLength === 0 ? 'charts empty' : 'new historical data loaded',
            currentDataPoints: currentDataLength,
            historicalDataPoints: dataHistory.length
        });
        
        // Clear existing chart data to populate with historical data
        charts.temperature.data.datasets[0].data = [];
        charts.humidity.data.datasets[0].data = [];
        charts.wind.data.datasets[0].data = [];
        charts.rain.data.datasets[0].data = [];
        charts.pressure.data.datasets[0].data = [];
        charts.light.data.datasets[0].data = [];
        if (charts.uv && charts.uv.data) {
            charts.uv.data.datasets[0].data = [];
        }
    } else {
        debugLog(logLevels.INFO, 'Skipping historical data population - charts already have live data', {
            currentDataPoints: currentDataLength,
            historicalDataPoints: dataHistory.length
        });
        return; // Don't overwrite existing live data
    }

    // Process each historical data point
    for (let i = 0; i < dataHistory.length; i++) {
        const obs = dataHistory[i];
        
        // Use actual timestamp from historical data if available, otherwise create backwards timeline
        let timestamp;
        if (obs.lastUpdate) {
            // Use the actual historical timestamp (weather API format)
            timestamp = new Date(obs.lastUpdate);
        } else if (obs.timestamp) {
            // Use the timestamp field (history API format - Unix timestamp in seconds)
            timestamp = new Date(obs.timestamp * 1000);
        } else {
            // Fallback to backwards timeline for generated data (for compatibility)
            const now = new Date();
            const secondsBack = (dataHistory.length - i - 1) * 10; // 10 seconds between each point
            timestamp = new Date(now.getTime() - (secondsBack * 1000));
        }

        debugLog(logLevels.DEBUG, `Historical data point ${i + 1}/${dataHistory.length}`, {
            timestamp: timestamp.toISOString(),
            hasActualTimestamp: !!(obs.lastUpdate || obs.timestamp),
            temperature: obs.temperature || obs.air_temperature,
            rain: obs.rainRate
        });

        /**
         * Defensive normalization for historical observation fields.
         * @param {*} v Candidate value.
         * @param {number} [fallback=0] Value used when `v` is not a finite number.
         * @returns {number}
         */
        const safeNumber = (v, fallback = 0) => (typeof v === 'number' && Number.isFinite(v)) ? v : fallback;

        // Temperature - handle both formats (temperature or air_temperature)
        let tempValue = safeNumber(obs.temperature || obs.air_temperature, 0);
        try { if (units.temperature === 'fahrenheit') tempValue = celsiusToFahrenheit(tempValue); } catch (e) { debugLog(logLevels.ERROR, 'Temperature conversion failed for historical point', { error: e.message }); }
        if (charts.temperature && charts.temperature.data && charts.temperature.data.datasets && charts.temperature.data.datasets[0]) {
            charts.temperature.data.datasets[0].data.push({ x: timestamp, y: tempValue });
        }

        // Humidity - handle both formats (humidity or relative_humidity)
        const humidityVal = safeNumber(obs.humidity || obs.relative_humidity, 0);
        if (charts.humidity && charts.humidity.data && charts.humidity.data.datasets && charts.humidity.data.datasets[0]) {
            charts.humidity.data.datasets[0].data.push({ x: timestamp, y: humidityVal });
        }

        // Wind - handle both formats (windSpeed or wind_avg)
        let windValue = safeNumber(obs.windSpeed || obs.wind_avg, 0);
        try { if (units.wind === 'kph') windValue = mphToKph(windValue); } catch (e) { debugLog(logLevels.ERROR, 'Wind conversion failed for historical point', { error: e.message }); }
        if (charts.wind && charts.wind.data && charts.wind.data.datasets && charts.wind.data.datasets[0]) {
            charts.wind.data.datasets[0].data.push({ x: timestamp, y: windValue });
        }

        // Rain - now using intensity (rate) and accumulation
        let rainRate = safeNumber(obs.rainRate, 0);
        // Convert to inches/hr if user prefers inches
        if (units.rain === 'inches') {
            rainRate = mmToInches(rainRate);
        }
        if (charts.rain && charts.rain.data && charts.rain.data.datasets && charts.rain.data.datasets[0]) {
            charts.rain.data.datasets[0].data.push({ x: timestamp, y: rainRate });
            if (i < 3 || i === dataHistory.length - 1) {
                debugLog(logLevels.INFO, `Rain data point ${i}`, {
                    timestamp: timestamp.toISOString(),
                    rainRate: rainRate,
                    unit: units.rain,
                    chartDataLength: charts.rain.data.datasets[0].data.length
                });
            }
        } else {
            debugLog(logLevels.ERROR, `Rain chart not ready for data point ${i}`, {
                hasChart: !!charts.rain,
                hasData: charts.rain ? !!charts.rain.data : false,
                hasDatasets: charts.rain?.data ? !!charts.rain.data.datasets : false,
                hasDataset0: charts.rain?.data?.datasets ? !!charts.rain.data.datasets[0] : false
            });
        }

        // Pressure - handle both formats (pressure or station_pressure)
        let pressureValue = safeNumber(obs.pressure || obs.station_pressure, 0);
        try { if (units.pressure === 'inHg') pressureValue = mbToInHg(pressureValue); } catch (e) { debugLog(logLevels.ERROR, 'Pressure conversion failed for historical point', { error: e.message }); }
        if (charts.pressure && charts.pressure.data && charts.pressure.data.datasets && charts.pressure.data.datasets[0]) {
            charts.pressure.data.datasets[0].data.push({ x: timestamp, y: pressureValue });
        }

        // Light
        const illumVal = safeNumber(obs.illuminance, 0);
        if (charts.light && charts.light.data && charts.light.data.datasets && charts.light.data.datasets[0]) {
            charts.light.data.datasets[0].data.push({ x: timestamp, y: illumVal });
        }

        // UV (if chart exists)
        const uvVal = safeNumber(obs.uv, 0);
        if (charts.uv && charts.uv.data && charts.uv.data.datasets && charts.uv.data.datasets[0]) {
            charts.uv.data.datasets[0].data.push({ x: timestamp, y: uvVal });
        }
    }

    // Ensure all datasets are validated and sorted after population
    ['temperature','humidity','wind','rain','pressure','light','uv'].forEach(name => {
        const chart = charts[name];
        if (chart && chart.data && chart.data.datasets && chart.data.datasets[0]) {
            validateAndSortChartData(chart);
            // Update average/trend where applicable
            if (chart === charts.temperature || chart === charts.humidity || chart === charts.wind || chart === charts.rain || chart === charts.pressure) {
                updateAverageLine(chart, chart.data.datasets[0].data);
                if (chart === charts.pressure) updateTrendLine(chart, chart.data.datasets[0].data);
            }
        }
    });

    // Update averages and trend lines for all charts
    updateAverageAndTrendLines();

    // Update all charts
    charts.temperature.update();
    charts.humidity.update();
    charts.wind.update();
    charts.rain.update();
    charts.pressure.update();
    charts.light.update();
    if (charts.uv && charts.uv.data) {
        charts.uv.update();
    }

    debugLog(logLevels.INFO, 'Charts populated with historical data', {
        temperaturePoints: charts.temperature.data.datasets[0].data.length,
        humidityPoints: charts.humidity.data.datasets[0].data.length,
        windPoints: charts.wind.data.datasets[0].data.length,
        rainPoints: charts.rain.data.datasets[0].data.length,
        pressurePoints: charts.pressure.data.datasets[0].data.length,
        lightPoints: charts.light.data.datasets[0].data.length,
        uvPoints: charts.uv && charts.uv.data ? charts.uv.data.datasets[0].data.length : 0
    });
}

/**
 * Recompute derived lines for the dashboard charts after a bulk load.
 *
 * Validates and sorts each series, sets the average lines for temperature,
 * humidity, wind and pressure, the pressure trend, and replaces rain dataset 1
 * with the accumulation integrated from the rain-rate series (rate x elapsed
 * hours, in display units). Light and UV naturally go to zero at night, so they
 * get no average.
 * @returns {void}
 * @throws {TypeError} If any of those charts has not been created.
 */
function updateAverageAndTrendLines() {
    // Update temperature average
    if (charts.temperature.data.datasets[0].data.length > 0) {
        validateAndSortChartData(charts.temperature);
        const tempAvg = calculateAverage(charts.temperature.data.datasets[0].data);
        updateAverageLine(charts.temperature, charts.temperature.data.datasets[0].data);
    }

    // Update humidity average
    if (charts.humidity.data.datasets[0].data.length > 0) {
        validateAndSortChartData(charts.humidity);
        const humidityAvg = calculateAverage(charts.humidity.data.datasets[0].data);
        updateAverageLine(charts.humidity, charts.humidity.data.datasets[0].data);
    }

    // Update wind average
    if (charts.wind.data.datasets[0].data.length > 0) {
        debugLog(logLevels.INFO, '🌬️ BEFORE wind validation - data points:', charts.wind.data.datasets[0].data.length);
        validateAndSortChartData(charts.wind);
        debugLog(logLevels.INFO, '🌬️ AFTER wind validation - data points:', charts.wind.data.datasets[0].data.length);
        const windAvg = calculateAverage(charts.wind.data.datasets[0].data);
        debugLog(logLevels.INFO, '🌬️ BEFORE wind updateAverageLine - avg:', windAvg);
        updateAverageLine(charts.wind, charts.wind.data.datasets[0].data);
        debugLog(logLevels.INFO, '🌬️ AFTER wind updateAverageLine - avg points:', charts.wind.data.datasets[1].data.length);
    }

    // Update rain - calculate accumulation from intensity data
    if (charts.rain.data.datasets[0].data.length > 0) {
        validateAndSortChartData(charts.rain);
        
        debugLog(logLevels.INFO, 'Calculating rain accumulation', {
            intensityDataPoints: charts.rain.data.datasets[0].data.length,
            hasAccumulationDataset: !!charts.rain.data.datasets[1]
        });
        
        // Calculate cumulative accumulation from intensity data
        let cumulativeRain = 0;
        const accumulationData = [];
        for (let i = 0; i < charts.rain.data.datasets[0].data.length; i++) {
            const point = charts.rain.data.datasets[0].data[i];
            if (i > 0) {
                const prevPoint = charts.rain.data.datasets[0].data[i - 1];
                const timeDiffHours = (point.x - prevPoint.x) / 3600000; // Convert ms to hours
                // point.y is already in user's preferred units (mm/hr or in/hr)
                cumulativeRain += point.y * timeDiffHours; // rate * time = accumulation
            }
            // cumulativeRain is already in user's preferred units
            accumulationData.push({ x: point.x, y: cumulativeRain });
        }
        if (charts.rain.data.datasets[1]) {
            charts.rain.data.datasets[1].data = accumulationData;
            debugLog(logLevels.INFO, 'Rain accumulation calculated', {
                accumulationDataPoints: accumulationData.length,
                finalAccumulation: accumulationData[accumulationData.length - 1]?.y,
                unit: units.rain
            });
        } else {
            debugLog(logLevels.ERROR, 'Rain accumulation dataset not found');
        }
    }

    // Update pressure average and trend
    if (charts.pressure.data.datasets[0].data.length > 0) {
        validateAndSortChartData(charts.pressure);
        const pressureAvg = calculateAverage(charts.pressure.data.datasets[0].data);
        updateAverageLine(charts.pressure, charts.pressure.data.datasets[0].data);
        updateTrendLine(charts.pressure, charts.pressure.data.datasets[0].data);
    }

    // Light data naturally goes to zero at night - no average needed

    // UV data naturally goes to zero at night - no average needed
}

/**
 * Render the HomeKit card: bridge status, accessory count, name and PIN, the
 * connection-info and technical-details rows (shown as `N/A` when the bridge is
 * off), the pairing QR code, and the accessories list.
 * @param {Object} status Parsed `/api/status` response; reads `status.homekit`.
 * @returns {void}
 */
function updateHomekitStatus(status) {
    // Update HomeKit status
    const homekitStatus = document.getElementById('homekit-status');
    const homekitAccessories = document.getElementById('homekit-accessories');
    const homekitBridge = document.getElementById('homekit-bridge');
    const homekitPin = document.getElementById('homekit-pin');

    const hk = status.homekit || {};
    if (homekitStatus) {
        if (hk.bridge) {
            homekitStatus.textContent = '✅ Active';
            homekitStatus.style.color = '#28a745';
        } else if (hk.status && hk.status.includes('Disabled')) {
            homekitStatus.textContent = '❌ Disabled';
            homekitStatus.style.color = '#dc3545';
        } else {
            homekitStatus.textContent = '⚠️ Inactive';
            homekitStatus.style.color = '#ffc107';
        }
    }
    if (homekitAccessories) homekitAccessories.textContent = hk.accessories || '--';
    if (homekitBridge) homekitBridge.textContent = hk.name || '--';
    if (homekitPin) homekitPin.textContent = hk.pin || '--';

    // Update Connection Info fields
    const setupCode = document.getElementById('homekit-setup-code');
    const pairedDevices = document.getElementById('homekit-paired-devices');
    const reachability = document.getElementById('homekit-reachability');
    const lastRequest = document.getElementById('homekit-last-request');
    
    if (setupCode) setupCode.textContent = hk.bridge ? (hk.setupCode || '--') : 'N/A';
    if (pairedDevices) pairedDevices.textContent = hk.bridge ? (hk.pairedDevices || 'Unknown') : 'N/A';
    
    // Generate QR code for HomeKit pairing
    if (hk.bridge && hk.pin) {
        generateHomekitQRCode(hk.pin);
    }
    if (reachability) {
        if (hk.bridge) {
            const reachable = hk.reachability !== false;
            reachability.textContent = reachable ? '✓ Reachable' : '✗ Unreachable';
            reachability.style.color = reachable ? '#28a745' : '#dc3545';
        } else {
            reachability.textContent = 'N/A';
            reachability.style.color = '#6c757d';
        }
    }
    if (lastRequest) lastRequest.textContent = hk.bridge ? (hk.lastRequest || 'Active') : 'N/A';
    
    // Update Technical Details fields
    const bridgeId = document.getElementById('homekit-bridge-id');
    const manufacturer = document.getElementById('homekit-manufacturer');
    const model = document.getElementById('homekit-model');
    const firmware = document.getElementById('homekit-firmware');
    const port = document.getElementById('homekit-port');
    const hapVersion = document.getElementById('homekit-hap-version');
    const configNumber = document.getElementById('homekit-config-number');
    const category = document.getElementById('homekit-category');
    const pairedUptime = document.getElementById('homekit-paired-uptime');
    
    if (bridgeId) bridgeId.textContent = hk.bridge ? (hk.bridgeId || '--') : 'N/A';
    if (manufacturer) manufacturer.textContent = hk.bridge ? (hk.manufacturer || '--') : 'N/A';
    if (model) model.textContent = hk.bridge ? (hk.model || '--') : 'N/A';
    if (firmware) firmware.textContent = hk.bridge ? (hk.firmware || '--') : 'N/A';
    if (port) port.textContent = hk.bridge ? (hk.port || '--') : 'N/A';
    if (hapVersion) hapVersion.textContent = hk.bridge ? (hk.hapVersion || '--') : 'N/A';
    if (configNumber) configNumber.textContent = hk.bridge ? (hk.configNumber || '--') : 'N/A';
    if (category) category.textContent = hk.bridge ? (hk.category || '--') : 'N/A';
    if (pairedUptime) pairedUptime.textContent = hk.bridge ? (hk.pairedUptime || 'Not yet paired') : 'N/A';

    // Update accessories list with all sensors info
    updateAccessoriesList(hk.accessoryNames || [], hk.allSensors || []);
    
    debugLog(logLevels.DEBUG, 'Status display update completed', {
        tempestConnected: status.connected,
        homekitActive: hk.bridge,
        accessoryCount: hk.accessories
    });
}

/**
 * Generate the HomeKit pairing QR code on the `#homekit-qr-code` canvas (200x200 px).
 *
 * Encodes `X-HM://<payload>` with the global `qrcode` library when it is loaded,
 * otherwise (or on any error) draws {@link generateFallbackQRCode}'s placeholder.
 * @param {string} setupCode HomeKit setup code (8 digits, dashes allowed).
 * @returns {void}
 */
function generateHomekitQRCode(setupCode) {
    const canvas = document.getElementById('homekit-qr-code');
    if (!canvas) return;

    const ctx = canvas.getContext('2d');
    const size = 200; // QR code size in pixels
    canvas.width = size;
    canvas.height = size;

    // Clear canvas
    ctx.fillStyle = 'white';
    ctx.fillRect(0, 0, size, size);

    try {
        // Calculate HomeKit setup payload
        const payload = calculateHomekitPayload(setupCode);
        const qrContent = `X-HM://${payload}`;

        debugLog(logLevels.DEBUG, 'Generating QR code for:', qrContent);

        // Use qrcode library if available, otherwise fallback
        if (typeof qrcode !== 'undefined') {
            // Create QR code object
            const qr = qrcode(0, 'M'); // Error correction level M
            qr.addData(qrContent);
            qr.make();

            // Calculate module size to fit in canvas
            const moduleCount = qr.getModuleCount();
            const moduleSize = size / moduleCount;

            // Clear canvas with white background
            ctx.fillStyle = 'white';
            ctx.fillRect(0, 0, size, size);

            // Draw QR code modules
            ctx.fillStyle = 'black';
            for (let row = 0; row < moduleCount; row++) {
                for (let col = 0; col < moduleCount; col++) {
                    if (qr.isDark(row, col)) {
                        ctx.fillRect(
                            col * moduleSize,
                            row * moduleSize,
                            moduleSize,
                            moduleSize
                        );
                    }
                }
            }

            debugLog(logLevels.DEBUG, 'HomeKit QR code generated successfully');
        } else {
            // Fallback to manual generation
            debugLog(logLevels.WARN, 'QRCode library not available, using fallback');
            generateFallbackQRCode(canvas, setupCode);
        }
    } catch (error) {
        debugLog(logLevels.ERROR, 'Failed to generate HomeKit QR code:', error);
        // Fallback to a simple pattern
        generateFallbackQRCode(canvas, setupCode);
    }
}

/**
 * Calculate the HomeKit setup payload according to the specification.
 *
 * Intended 64-bit layout, as listed in the code: version (3 bits) 0, reserved (4)
 * 0, category (8) 2 = bridge, flags (8) 0, setup code (27), reserved (4) 0,
 * reserved (10) 0. Built with BigInt from LSB to MSB.
 *
 * @param {string} setupCode Setup code; non-digits are stripped.
 * @returns {string} The payload as a decimal string.
 * @throws {Error} If the code does not contain exactly 8 digits.
 */
function calculateHomekitPayload(setupCode) {
    // Extract digits from setup code (remove dashes and non-digits)
    const digits = setupCode.replace(/[^0-9]/g, '');
    if (digits.length !== 8) {
        throw new Error('Invalid setup code: must be 8 digits');
    }

    const setupValue = parseInt(digits, 10);
    if (setupValue < 0 || setupValue > 99999999) {
        throw new Error('Invalid setup code range');
    }

    // HomeKit setup payload format (64-bit):
    // Version (3 bits): 0
    // Reserved (4 bits): 0
    // Category (8 bits): 2 (bridge)
    // Flags (8 bits): 0
    // Setup code (27 bits): encoded from 8 digits
    // Reserved (4 bits): 0
    // Reserved (10 bits): 0

    let payload = 0n;

    // Build from LSB to MSB (correct bit positioning)
    payload |= 0n;  // Version (3 bits): 0
    payload <<= 4n; // Reserved (4 bits): 0
    payload |= 2n;  // Category (8 bits): 2 (bridge)
    payload <<= 8n; // Flags (8 bits): 0
    payload |= 0n;  // Flags
    payload <<= 27n; // Setup code (27 bits) - FIXED: was <<= 8n
    payload |= BigInt(setupValue);
    payload <<= 4n; // Reserved (4 bits): 0
    // Final 10 bits remain 0 (reserved)

    return payload.toString(10);
}



/**
 * Fallback QR code generation for errors: draws a 21x21 placeholder pattern that
 * darkens one cell per setup-code digit greater than 5. It is not a scannable QR code.
 * @param {HTMLCanvasElement} canvas Target canvas.
 * @param {string} setupCode HomeKit setup code.
 * @returns {void}
 */
function generateFallbackQRCode(canvas, setupCode) {
    const ctx = canvas.getContext('2d');
    const size = canvas.width;
    const cellSize = size / 21;

    // Clear canvas
    ctx.fillStyle = 'white';
    ctx.fillRect(0, 0, size, size);

    // Simple pattern based on setup code
    const codeStr = setupCode.replace(/[^0-9]/g, '');
    ctx.fillStyle = 'black';

    for (let i = 0; i < Math.min(codeStr.length, 21 * 21); i++) {
        const digit = parseInt(codeStr[i % codeStr.length]);
        if (digit > 5) { // Arbitrary threshold
            const x = i % 21;
            const y = Math.floor(i / 21);
            ctx.fillRect(x * cellSize, y * cellSize, cellSize, cellSize);
        }
    }

    debugLog(logLevels.WARN, 'Using fallback QR code generation');
}

/**
 * Render the HomeKit accessories list: every configured sensor with its icon,
 * enabled ones first and marked Active, the rest marked Disabled.
 *
 * A sensor counts as enabled when any accessory name contains it or vice versa.
 * @param {Array<string>} accessoryNames Names of accessories the bridge exposes.
 * @param {Array<string>} allSensors All configurable sensor names.
 * @returns {void}
 */
function updateAccessoriesList(accessoryNames, allSensors) {
    debugLog(logLevels.DEBUG, 'Updating accessories list', { accessoryNames, allSensors });
    
    const accessoriesList = document.getElementById('accessories-list');
    if (!accessoriesList) {
        debugLog(logLevels.WARN, 'Accessories list element not found');
        return;
    }
    
    accessoriesList.innerHTML = '';

    // If no configured sensors at all
    if (!allSensors || allSensors.length === 0) {
        accessoriesList.innerHTML = '<div class="accessory-item">No sensors configured</div>';
        debugLog(logLevels.DEBUG, 'No sensors configured');
        return;
    }

    // Define sensor icons mapping
    const sensorIcons = {
        'Temperature': '🌡️',
        'Humidity': '💧',
        'Light': '☀️',
        'UV': '🌞',
        'Wind Speed': '🌬️',
        'Wind Direction': '🧭',
        'Rain': '🌧️',
        'Pressure': '📊',
        'Lightning': '⚡'
    };

    // Determine which sensors are enabled based on accessoryNames
    const enabledSensors = [];
    const disabledSensors = [];

    allSensors.forEach(sensorName => {
        const isEnabled = accessoryNames && accessoryNames.some(name => name.includes(sensorName) || sensorName.includes(name));
        const sensor = {
            name: sensorName === 'UV' ? 'UV Index' : sensorName,
            icon: sensorIcons[sensorName] || '❓',
            key: sensorName,
            enabled: isEnabled
        };
        
        if (isEnabled) {
            enabledSensors.push(sensor);
        } else {
            disabledSensors.push(sensor);
        }
    });

    // Sort enabled sensors to the top, then disabled
    const sortedSensors = [...enabledSensors, ...disabledSensors];

    if (sortedSensors.length === 0) {
        accessoriesList.innerHTML = '<div class="accessory-item">No sensors configured</div>';
        return;
    }

    sortedSensors.forEach(sensor => {
        const accessoryDiv = document.createElement('div');
        accessoryDiv.className = 'accessory-item' + (sensor.enabled ? '' : ' disabled');

        const statusClass = sensor.enabled ? 'enabled' : 'disabled';
        const statusText = sensor.enabled ? 'Active' : 'Disabled';
        const nameClass = sensor.enabled ? '' : ' disabled';

        accessoryDiv.innerHTML = 
            '<span class="accessory-icon">' + sensor.icon + '</span>' +
            '<span class="accessory-name' + nameClass + '">' + sensor.name + '</span>' +
            '<span class="accessory-status ' + statusClass + '">' + statusText + '</span>';

        accessoriesList.appendChild(accessoryDiv);
    });
    
    debugLog(logLevels.DEBUG, 'Accessories list updated', {
        totalSensors: sortedSensors.length,
        enabled: enabledSensors.length,
        disabled: disabledSensors.length
    });
}

/**
 * Expand or collapse the HomeKit accessories section and flip its ▶/▼ icon.
 * @returns {void}
 */
function toggleAccessoriesExpansion() {
    const expandedDiv = document.getElementById('accessories-expanded');
    const expandIcon = document.getElementById('accessories-expand-icon');

    if (expandedDiv && expandIcon) {
        const isExpanded = expandedDiv.style.display !== 'none' && expandedDiv.style.display !== '';
        
        if (!isExpanded) {
            expandedDiv.style.display = 'block';
            expandIcon.textContent = '▼';
        } else {
            expandedDiv.style.display = 'none';
            expandIcon.textContent = '▶';
        }
        
        debugLog(logLevels.DEBUG, 'Accessories expansion toggled', { expanded: !isExpanded });
    }
}

/**
 * Expand or collapse the Tempest device status section and flip its ▶/▼ icon.
 * @returns {void}
 */
function toggleDeviceStatusExpansion() {
    const expandedDiv = document.getElementById('device-status-expanded');
    const expandIcon = document.getElementById('device-status-expand-icon');

    if (expandedDiv && expandIcon) {
        const isExpanded = expandedDiv.style.display !== 'none' && expandedDiv.style.display !== '';
        
        if (!isExpanded) {
            expandedDiv.style.display = 'block';
            expandIcon.textContent = '▼';
        } else {
            expandedDiv.style.display = 'none';
            expandIcon.textContent = '▶';
        }
        
        debugLog(logLevels.DEBUG, 'Device status expansion toggled', { expanded: !isExpanded });
    }
}

/**
 * Expand or collapse the Tempest hub status section and flip its ▶/▼ icon.
 * @returns {void}
 */
function toggleHubStatusExpansion() {
    const expandedDiv = document.getElementById('hub-status-expanded');
    const expandIcon = document.getElementById('hub-status-expand-icon');

    if (expandedDiv && expandIcon) {
        const isExpanded = expandedDiv.style.display !== 'none' && expandedDiv.style.display !== '';
        
        if (!isExpanded) {
            expandedDiv.style.display = 'block';
            expandIcon.textContent = '▼';
        } else {
            expandedDiv.style.display = 'none';
            expandIcon.textContent = '▶';
        }
        
        debugLog(logLevels.DEBUG, 'Hub status expansion toggled', { expanded: !isExpanded });
    }
}

/**
 * Expand or collapse the HomeKit connection info section and flip its ▶/▼ icon.
 * @returns {void}
 */
function toggleHomekitConnectionExpansion() {
    const expandedDiv = document.getElementById('homekit-connection-expanded');
    const expandIcon = document.getElementById('homekit-connection-expand-icon');

    if (expandedDiv && expandIcon) {
        const isExpanded = expandedDiv.style.display !== 'none' && expandedDiv.style.display !== '';
        
        if (!isExpanded) {
            expandedDiv.style.display = 'block';
            expandIcon.textContent = '▼';
        } else {
            expandedDiv.style.display = 'none';
            expandIcon.textContent = '▶';
        }
        
        debugLog(logLevels.DEBUG, 'HomeKit connection expansion toggled', { expanded: !isExpanded });
    }
}

/**
 * Expand or collapse the HomeKit technical details section and flip its ▶/▼ icon.
 * @returns {void}
 */
function toggleHomekitTechnicalExpansion() {
    const expandedDiv = document.getElementById('homekit-technical-expanded');
    const expandIcon = document.getElementById('homekit-technical-expand-icon');

    if (expandedDiv && expandIcon) {
        const isExpanded = expandedDiv.style.display !== 'none' && expandedDiv.style.display !== '';
        
        if (!isExpanded) {
            expandedDiv.style.display = 'block';
            expandIcon.textContent = '▼';
        } else {
            expandedDiv.style.display = 'none';
            expandIcon.textContent = '▶';
        }
        
        debugLog(logLevels.DEBUG, 'HomeKit technical expansion toggled', { expanded: !isExpanded });
    }
}

/**
 * Toggle compact mode for the Tempest card, persist it to localStorage
 * (`tempest-compact-mode`) and update the toggle button's active state.
 * @returns {void}
 */
function toggleCompactMode() {
    const tempestCard = document.getElementById('tempest-card');
    if (tempestCard) {
        tempestCard.classList.toggle('compact');
        const isCompact = tempestCard.classList.contains('compact');
        localStorage.setItem('tempest-compact-mode', isCompact ? 'true' : 'false');
        
        // Update toggle button appearance
        const toggleButton = document.getElementById('tempest-compact-toggle');
        if (toggleButton) {
            if (isCompact) {
                toggleButton.classList.add('active');
            } else {
                toggleButton.classList.remove('active');
            }
        }
        
        debugLog(logLevels.DEBUG, 'Compact mode toggled', { compact: isCompact });
    }
}

/**
 * Toggle compact mode for the alarm card, persist it to localStorage
 * (`alarm-compact-mode`) and update the toggle button's active state.
 * @returns {void}
 */
function toggleAlarmCompactMode() {
    const alarmCard = document.getElementById('alarm-card');
    if (alarmCard) {
        alarmCard.classList.toggle('compact');
        const isCompact = alarmCard.classList.contains('compact');
        localStorage.setItem('alarm-compact-mode', isCompact ? 'true' : 'false');
        
        // Update toggle button appearance
        const toggleButton = document.getElementById('alarm-compact-toggle');
        if (toggleButton) {
            if (isCompact) {
                toggleButton.classList.add('active');
            } else {
                toggleButton.classList.remove('active');
            }
        }
        
        debugLog(logLevels.DEBUG, 'Alarm compact mode toggled', { compact: isCompact });
    }
}

/**
 * Update the battery indicator with colour coding: good above 2.5 V, fair above
 * 2.3 V, low otherwise; hidden when the voltage cannot be parsed.
 * @param {number|string} voltage Battery voltage, or a string such as `"2.45V"`.
 * @returns {void}
 */
function updateBatteryIndicator(voltage) {
    const indicator = document.getElementById('tempest-battery-indicator');
    if (!indicator) return;
    
    // Remove all status classes
    indicator.classList.remove('good', 'fair', 'low');
    
    // Parse voltage if it's a string like "2.45V"
    let voltageNum = typeof voltage === 'string' ? parseFloat(voltage) : voltage;
    
    if (isNaN(voltageNum)) {
        indicator.style.display = 'none';
        return;
    }
    
    indicator.style.display = 'inline-block';
    
    // Color code based on voltage
    if (voltageNum > 2.5) {
        indicator.classList.add('good');
    } else if (voltageNum > 2.3) {
        indicator.classList.add('fair');
    } else {
        indicator.classList.add('low');
    }
}

/**
 * Create four signal strength bars, lighting 4/3/2/1 of them for RSSI at or above
 * -50/-60/-70 dBm or below.
 * @param {number|string} rssi RSSI in dBm, or a string such as `"-45 dBm (Excellent)"`.
 * @param {string} containerId Id of the element that receives the bars.
 * @returns {void}
 */
function createSignalBars(rssi, containerId) {
    const container = document.getElementById(containerId);
    if (!container) return;
    
    // Clear existing bars
    container.innerHTML = '';
    
    // Parse RSSI if it's a string like "-45 dBm (Excellent)"
    let rssiNum = typeof rssi === 'string' ? parseInt(rssi) : rssi;
    
    if (isNaN(rssiNum)) {
        return;
    }
    
    // Determine signal quality
    let quality;
    let activeBars;
    if (rssiNum >= -50) {
        quality = 'excellent';
        activeBars = 4;
    } else if (rssiNum >= -60) {
        quality = 'good';
        activeBars = 3;
    } else if (rssiNum >= -70) {
        quality = 'fair';
        activeBars = 2;
    } else {
        quality = 'poor';
        activeBars = 1;
    }
    
    // Create 4 bars
    for (let i = 1; i <= 4; i++) {
        const bar = document.createElement('span');
        bar.className = 'signal-bar';
        if (i <= activeBars) {
            bar.classList.add('active', quality);
        }
        container.appendChild(bar);
    }
}

/**
 * Attach an event listener by element id, waiting for the element if it is not in
 * the DOM yet (enhanced DOM-ready checking with debug logging).
 *
 * If the element is missing, a MutationObserver and up to 10 retries 300 ms apart
 * both try to attach the handler.
 * @param {string} elementId Target element id.
 * @param {string} event Event type, e.g. `'click'`.
 * @param {EventListener} handler Listener to attach.
 * @param {string} [description=''] Purpose, used only in log messages.
 * @returns {void}
 */
function attachEventListener(elementId, event, handler, description = '') {
    const element = document.getElementById(elementId);
    if (element) {
        element.addEventListener(event, handler);
        debugLog(logLevels.DEBUG, `Event listener attached: ${elementId}`, {
            event: event,
            description: description
        });
    } else {
        debugLog(logLevels.WARN, `Element not found for event listener: ${elementId}`, {
            description: description,
            retrying: true
        });
        
        // Use MutationObserver to wait for the element to appear
        const observer = new MutationObserver((mutations, obs) => {
            const targetElement = document.getElementById(elementId);
            if (targetElement) {
                targetElement.addEventListener(event, handler);
                debugLog(logLevels.INFO, `Event listener attached via MutationObserver: ${elementId}`, {
                    event: event,
                    description: description
                });
                obs.disconnect();
                return;
            }
        });
        
        observer.observe(document.body, {
            childList: true,
            subtree: true
        });
        
        // Fallback: Try again after delays with more attempts
        let retryCount = 0;
        const maxRetries = 10;
        const retryDelay = 300; // 300ms delay
        
        /**
         * One delayed retry of the attachment; reschedules itself until the element
         * appears or the retry limit is reached, then disconnects the observer.
         * @returns {void}
         */
        const retryAttachment = () => {
            retryCount++;
            const retryElement = document.getElementById(elementId);
            if (retryElement) {
                retryElement.addEventListener(event, handler);
                debugLog(logLevels.INFO, `Event listener attached on retry ${retryCount}: ${elementId}`, {
                    event: event,
                    description: description
                });
                observer.disconnect(); // Stop observing if we succeed
            } else if (retryCount < maxRetries) {
                debugLog(logLevels.WARN, `Retry ${retryCount}/${maxRetries} failed for: ${elementId}`, {
                    nextRetryIn: retryDelay + 'ms',
                    allElements: Array.from(document.querySelectorAll('[id]')).map(el => el.id)
                });
                setTimeout(retryAttachment, retryDelay);
            } else {
                debugLog(logLevels.ERROR, `Failed to attach event listener after ${maxRetries} retries: ${elementId}`, {
                    event: event,
                    description: description,
                    elementExists: !!document.getElementById(elementId),
                    bodyHTML: document.body.innerHTML.includes(elementId),
                    searchResult: document.body.innerHTML.indexOf(elementId)
                });
                observer.disconnect(); // Stop observing after max retries
            }
        };
        
        setTimeout(retryAttachment, retryDelay);
    }
}

/**
 * Initialize when the DOM is ready.
 *
 * Enables debug mode from `?loglevel=debug`, loads the server units, starts the
 * first fetches, initializes the charts (directly on the popout page; on the
 * dashboard via a resilient retry loop that can load the vendored Chart.js), wires
 * the card toggles and tooltip handlers, restores compact modes, and starts
 * polling: weather every 5 s, status and alarms every 10 s.
 */
document.addEventListener('DOMContentLoaded', function() {
    debugLog(logLevels.INFO, 'DOM Content Loaded - Initializing dashboard');
    
    // Check if debug mode should be enabled from URL or localStorage
    const urlParams = new URLSearchParams(window.location.search);
    if (urlParams.get('loglevel') === 'debug') {
        localStorage.setItem('loglevel', 'debug');
        DEBUG_MODE = true;
        debugLog(logLevels.INFO, 'Debug mode enabled via URL parameter');
    }
    
    // IMMEDIATE DOM INSPECTION for pressure-info-icon
    console.log('🔍 IMMEDIATE DOM CHECK:');
    console.log('- pressure-info-icon exists:', !!document.getElementById('pressure-info-icon'));
    console.log('- pressure-card exists:', !!document.getElementById('pressure-card'));
    console.log('- pressure-unit exists:', !!document.getElementById('pressure-unit'));
    
    const pressureUnit = document.getElementById('pressure-unit');
    if (pressureUnit) {
        console.log('- pressure-unit innerHTML:', pressureUnit.innerHTML);
        console.log('- pressure-unit children:', pressureUnit.children.length);
        for (let i = 0; i < pressureUnit.children.length; i++) {
            const child = pressureUnit.children[i];
            console.log(`  Child ${i}:`, child.tagName, child.id, child.className);
        }
    } else {
        console.log('- pressure-unit element not found!');
    }
    
    // Check all elements with "pressure" in ID
    const pressureElements = Array.from(document.querySelectorAll('[id*="pressure"]'));
    console.log('- All pressure-related elements:', pressureElements.map(el => el.id));
    
    // Check for info-icon class elements
    const infoIcons = Array.from(document.querySelectorAll('.info-icon'));
    console.log('- All info-icon elements:', infoIcons.map(el => el.id || el.tagName));
    
    console.log('🔍 DOM CHECK COMPLETE');
    
    // Detect if we're running on the standalone chart popout page which
    // contains only the chart canvas (`#chart-root`) and not the full
    // dashboard elements (e.g. `#temperature`). In that case skip dashboard
    // specific initialization (fetching/updating DOM elements, attaching
    // dashboard event listeners) since those elements don't exist.
    const isChartOnly = !!document.getElementById('chart-root') && !document.getElementById('temperature');

    // Load units configuration from server first, then update units
    loadUnitsConfig().then(() => {
        updateUnits();
    });

    if (!isChartOnly) {
        // Start data fetching immediately so the status UI updates even if Chart.js
        // is unavailable or takes time to load. This prevents a ReferenceError in
        // initCharts (when Chart is undefined) from blocking network calls and
        // leaving the page stuck on "Connecting to weather station...".
        try {
            console.log('🚀 DEBUG: Starting initial data fetch (before charts)');
            fetchWeather();
            fetchStatus();
            fetchAlarmStatus();
        } catch (e) {
            debugLog(logLevels.ERROR, 'Error triggering initial fetches', e);
        }
    } else {
        debugLog(logLevels.INFO, 'Detected chart-only popout page - skipping dashboard fetches and event listener attachments');
    }
    
    // Ensure pressure info icon has proper event listener attached initially
    const initialPressureInfoIcon = document.getElementById('pressure-info-icon');
    if (initialPressureInfoIcon) {
        initialPressureInfoIcon.addEventListener('click', function(e) {
            e.stopPropagation();
            togglePressureTooltip(e);
        });
        console.log('🔧 Initial setup - Attached click event listener to pressure info icon');
    }
    
    // Initialize charts, but be resilient if Chart.js is not yet loaded.
    // If Chart is undefined, attempt to load the local vendored copy and
    // initialize charts when it finishes loading. Meanwhile, data fetches
    // are already running so the UI won't be blocked.
    // For popout pages, we just call initCharts() directly since the canvas is already present
    if (isChartOnly) {
        debugLog(logLevels.INFO, 'Initializing popout chart for chart-only page');
        if (typeof Chart !== 'undefined') {
            try {
                initCharts();
                __chartsInitialized = true;
                debugLog(logLevels.INFO, 'Popout chart initialized successfully');
                
                // Load historical data for the popout chart
                loadHistoricalDataForPopout();
            } catch (e) {
                debugLog(logLevels.ERROR, 'Failed to initialize popout chart', e);
            }
        } else {
            debugLog(logLevels.ERROR, 'Chart.js not loaded - cannot initialize popout chart');
        }
    } else {
        // Only attempt the full chart initialization flow on the dashboard page.
        /**
         * Try to initialize the dashboard charts, recovering from common failures.
         *
         * Gives up after 8 attempts. On a "canvas is already in use" error it runs one
         * {@link destroyAllCharts} sweep and retries; if Chart.js is missing it loads
         * `/pkg/web/static/chart.umd.js` and the date-fns adapter dynamically; otherwise
         * it retries after 500 ms. Replays `window.__pendingHistoricalData` on success.
         * @returns {void}
         */
        (function initChartsResilient() {
        if (__chartsInitialized) return; // already initialized successfully

        __chartInitAttempts++;
        if (__chartInitAttempts > 8) {
            debugLog(logLevels.ERROR, 'Aborting chart initialization after multiple failed attempts', { attempts: __chartInitAttempts });
            return;
        }
        console.log('🚀 DEBUG: Attempting chart initialization (resilient) attempt=' + __chartInitAttempts);
        if (typeof Chart !== 'undefined' && !__chartsInitialized) {
            try {
                initCharts();
                    console.log('🚀 DEBUG: Charts initialized:', Object.keys(charts));
                    console.log('🚀 DEBUG: Temperature chart exists:', !!charts.temperature);
                    console.log('🚀 DEBUG: Rain chart exists:', !!charts.rain);
                    __chartsInitialized = true;
                    
                    // Process pending historical data if available
                    if (window.__pendingHistoricalData) {
                        debugLog(logLevels.INFO, 'Processing deferred historical data after chart initialization', {
                            dataPoints: window.__pendingHistoricalData.length
                        });
                        populateChartsWithHistoricalData(window.__pendingHistoricalData);
                        window.__pendingHistoricalData = null;
                    }
                    
                    trySetDashboardReady();
                return;
            } catch (e) {
                debugLog(logLevels.ERROR, 'Error during initCharts()', e);
                // If we detect the canvas-in-use problem, attempt a single global destroy sweep
                if (String(e).toLowerCase().includes('canvas is already in use') && !__didGlobalChartDestroy) {
                    debugLog(logLevels.WARN, 'Detected canvas-in-use error; performing one-time destroyAllCharts sweep before retry');
                    destroyAllCharts();
                    // Try once more
                    try {
                        initCharts();
                        __chartsInitialized = true;
                        debugLog(logLevels.INFO, 'Charts initialized after global destroy');
                        
                        // Process pending historical data if available
                        if (window.__pendingHistoricalData) {
                            debugLog(logLevels.INFO, 'Processing deferred historical data after chart recovery', {
                                dataPoints: window.__pendingHistoricalData.length
                            });
                            populateChartsWithHistoricalData(window.__pendingHistoricalData);
                            window.__pendingHistoricalData = null;
                        }
                        
                        return;
                    } catch (e2) {
                        debugLog(logLevels.ERROR, 'Second attempt to initCharts failed after destroyAllCharts', e2);
                    }
                }
            }
        }

        // If Chart is missing, dynamically load local vendored Chart.js and adapter
        if (typeof Chart === 'undefined' && !__chartVendorLoading) {
            debugLog(logLevels.WARN, 'Chart.js not found - dynamically loading local vendored copy');

            __chartVendorLoading = true;

            const vendorScript = document.createElement('script');
            vendorScript.src = '/pkg/web/static/chart.umd.js';
            vendorScript.async = false;
            /**
             * Runs after the vendored Chart.js loads; loads the date adapter next.
             * @returns {void}
             */
            vendorScript.onload = function() {
                debugLog(logLevels.INFO, 'Local Chart.js loaded, attempting to initialize charts');
                // Try to load adapter as well (if present)
                const adapter = document.createElement('script');
                adapter.src = '/pkg/web/static/chartjs-adapter-date-fns.bundle.min.js';
                adapter.async = false;
                /**
                 * Runs after the date adapter loads; initializes the charts and re-checks readiness.
                 * @returns {void}
                 */
                adapter.onload = function() {
                    try {
                        initCharts();
                        __chartsInitialized = true;
                        trySetDashboardReady();
                        console.log('🚀 DEBUG: Charts initialized after loading local Chart.js');
                    } catch (e) {
                        debugLog(logLevels.ERROR, 'initCharts failed after loading vendor scripts', e);
                    }
                };
                /**
                 * Adapter failed to load; still try to initialize the charts in case it isn't required.
                 * @param {Event|string} err Load error.
                 * @returns {void}
                 */
                adapter.onerror = function(err) {
                    debugLog(logLevels.ERROR, 'Failed to load local Chart adapter', err || {});
                    // Still try to initCharts in case adapter isn't required
                    try { initCharts(); } catch (e) { debugLog(logLevels.ERROR, 'initCharts fallback failed', e); }
                };
                document.head.appendChild(adapter);
            };
            /**
             * Vendored Chart.js failed to load; log it and allow another load attempt.
             * @param {Event|string} err Load error.
             * @returns {void}
             */
            vendorScript.onerror = function(err) {
                debugLog(logLevels.ERROR, 'Failed to load local Chart.js vendor script', err || {});
                __chartVendorLoading = false;
            };
            document.head.appendChild(vendorScript);
            return;
        }

        // As a final fallback, try again shortly
        setTimeout(initChartsResilient, 500);
        })();
    }

    // Attach event listeners with debug logging (skip if chart-only popout)
    debugLog(logLevels.DEBUG, 'Starting to attach event listeners');
    
    // Debug: Check if pressure elements exist and log all IDs
    const pressureCard = document.getElementById('pressure-card');
    const pressureInfoIcon = document.getElementById('pressure-info-icon');
    const allElements = Array.from(document.querySelectorAll('[id]'));
    const allIds = allElements.map(el => el.id);
    
    debugLog(logLevels.DEBUG, 'Pressure elements check', {
        pressureCard: !!pressureCard,
        pressureInfoIcon: !!pressureInfoIcon,
        pressureCardDisplay: pressureCard ? pressureCard.style.display : 'N/A',
        totalElementsWithIds: allElements.length,
        pressureRelatedIds: allIds.filter(id => id.includes('pressure')),
        allIds: allIds
    });
    
    if (!isChartOnly) {
        attachEventListener('accessories-row', 'click', toggleAccessoriesExpansion, 'Toggle accessories expansion');
        attachEventListener('device-status-row', 'click', toggleDeviceStatusExpansion, 'Toggle device status expansion');
        attachEventListener('hub-status-row', 'click', toggleHubStatusExpansion, 'Toggle hub status expansion');
        attachEventListener('homekit-connection-row', 'click', toggleHomekitConnectionExpansion, 'Toggle HomeKit connection info');
        attachEventListener('homekit-technical-row', 'click', toggleHomekitTechnicalExpansion, 'Toggle HomeKit technical details');
        attachEventListener('tempest-compact-toggle', 'click', toggleCompactMode, 'Toggle compact/detailed view mode');
        attachEventListener('alarm-compact-toggle', 'click', toggleAlarmCompactMode, 'Toggle alarm compact/detailed view mode');
        attachEventListener('lux-info-icon', 'click', toggleLuxTooltip, 'Show/hide lux information tooltip');
        attachEventListener('lux-tooltip-close', 'click', closeLuxTooltip, 'Close lux tooltip');
        attachEventListener('rain-info-icon', 'click', toggleRainTooltip, 'Show/hide rain information tooltip');
        attachEventListener('rain-tooltip-close', 'click', closeRainTooltip, 'Close rain tooltip');
        attachEventListener('humidity-info-icon', 'click', toggleHumidityTooltip, 'Show/hide humidity information tooltip');
        attachEventListener('humidity-tooltip-close', 'click', closeHumidityTooltip, 'Close humidity tooltip');
        attachEventListener('heat-index-info-icon', 'click', toggleHeatIndexTooltip, 'Show/hide heat index tooltip');
        attachEventListener('heat-index-tooltip-close', 'click', closeHeatIndexTooltip, 'Close heat index tooltip');
        attachEventListener('uv-info-icon', 'click', toggleUVTooltip, 'Show/hide UV information tooltip');
        attachEventListener('uv-tooltip-close', 'click', closeUVTooltip, 'Close UV tooltip');
        attachEventListener('pressure-info-icon', 'click', togglePressureTooltip, 'Show/hide pressure tooltip');
        attachEventListener('pressure-tooltip-close', 'click', closePressureTooltip, 'Close pressure tooltip');
    }

    // Global click handlers for closing tooltips
    document.addEventListener('click', handleLuxTooltipClickOutside);
    document.addEventListener('click', handleRainTooltipClickOutside);
    document.addEventListener('click', handleHumidityTooltipClickOutside);
    document.addEventListener('click', handleHeatIndexTooltipClickOutside);
    document.addEventListener('click', handleUVTooltipClickOutside);
    document.addEventListener('click', handlePressureTooltipClickOutside);
    
    debugLog(logLevels.DEBUG, 'All event listeners attached');
    
    // Restore compact mode from localStorage
    const isCompact = localStorage.getItem('tempest-compact-mode') === 'true';
    if (isCompact) {
        const tempestCard = document.getElementById('tempest-card');
        if (tempestCard) {
            tempestCard.classList.add('compact');
        }
        const toggleButton = document.getElementById('tempest-compact-toggle');
        if (toggleButton) {
            toggleButton.classList.add('active');
        }
    }

    // Restore alarm compact mode from localStorage
    const isAlarmCompact = localStorage.getItem('alarm-compact-mode') === 'true';
    if (isAlarmCompact) {
        const alarmCard = document.getElementById('alarm-card');
        if (alarmCard) {
            alarmCard.classList.add('compact');
        }
        const toggleButton = document.getElementById('alarm-compact-toggle');
        if (toggleButton) {
            toggleButton.classList.add('active');
        }
    }

    // Start data fetching - fetch status first to determine data source type
    debugLog(logLevels.INFO, 'Starting periodic data fetching (weather: 5s, status/alarms: 10s)');
    console.log('🚀 DEBUG: Starting initial data fetch');
    // Fetch status first to set currentDataSourceType before weather fetch attempts
    fetchStatus().then(() => fetchWeather());
    
    // Weather data updates every 5 seconds for real-time chart updates
    setInterval(() => {
        console.log('🚀 DEBUG: Periodic weather fetch triggered');
        debugLog(logLevels.DEBUG, 'Periodic weather fetch triggered');
        fetchWeather();
    }, 5000);
    
    // Status and alarm status updates every 10 seconds
    setInterval(() => {
        console.log('🚀 DEBUG: Periodic status/alarm fetch triggered');
        debugLog(logLevels.DEBUG, 'Periodic status/alarm fetch triggered');
        fetchStatus();
        fetchAlarmStatus();
    }, 10000);
    
    debugLog(logLevels.INFO, 'Dashboard initialization completed');
});

/**
 * Regenerate the weather data for testing (generated weather mode only).
 *
 * POSTs `/api/regenerate-weather`, briefly shows "Regenerated! Refreshing..." in
 * the station name and re-fetches weather and status after 1.5 s; on failure shows
 * "Regeneration failed" for 2 s. Called from the station name link that
 * {@link updateStatusDisplay} renders.
 * @async
 * @returns {Promise<void>}
 */
async function regenerateWeather() {
    try {
        debugLog(logLevels.INFO, 'Regenerating weather data...');
        
        const response = await fetch('/api/regenerate-weather', {
            method: 'POST',
            headers: {
                'Content-Type': 'application/json',
            }
        });
        
        if (!response.ok) {
            throw new Error(`Failed to regenerate weather: ${response.status}`);
        }
        
        const result = await response.json();
        debugLog(logLevels.INFO, 'Weather regenerated successfully', result);
        
        // Show a brief notification
        const stationElement = document.getElementById('tempest-station');
        if (stationElement) {
            const originalContent = stationElement.innerHTML;
            stationElement.innerHTML = '<span style="color: #28a745;">Regenerated! Refreshing...</span>';
            
            // Refresh the data after a short delay
            setTimeout(() => {
                stationElement.innerHTML = originalContent;
                fetchWeather();
                fetchStatus();
            }, 1500);
        }
        
    } catch (error) {
        debugLog(logLevels.ERROR, 'Failed to regenerate weather', error);
        console.error('Failed to regenerate weather:', error);
        
        // Show error notification
        const stationElement = document.getElementById('tempest-station');
        if (stationElement) {
            const originalContent = stationElement.innerHTML;
            stationElement.innerHTML = '<span style="color: #dc3545;">Regeneration failed</span>';
            setTimeout(() => {
                stationElement.innerHTML = originalContent;
            }, 2000);
        }
    }
}

// Debug function to test chart functionality
if (DEBUG_MODE) {
    /**
     * Debug function to test chart functionality: logs chart state and pushes a test
     * point (25) onto the temperature chart. Only defined when {@link DEBUG_MODE} is on at load.
     * @returns {void}
     */
    window.testCharts = function() {
        console.log('🔧 Testing charts functionality');
        console.log('Charts object:', charts);
        console.log('Chart.js available:', typeof Chart !== 'undefined');
        console.log('Temperature chart exists:', !!charts.temperature);
        console.log('Rain chart exists:', !!charts.rain);
        
        if (charts.temperature) {
            try {
                console.log('Temperature chart data length:', charts.temperature.data.datasets[0].data.length);
            } catch (e) { console.log('Temperature chart data not available yet'); }
        }
        
        if (charts.rain) {
            try {
                console.log('Rain chart data length:', charts.rain.data.datasets[0].data.length);
                console.log('Rain chart datasets:', charts.rain.data.datasets.length);
            } catch (e) { console.log('Rain chart data not available yet'); }
        }
        
        console.log('weatherData:', weatherData);
        
        // Try to add a test point
        if (weatherData && charts.temperature) {
            try {
                const testPoint = { x: new Date(), y: 25 };
                charts.temperature.data.datasets[0].data.push(testPoint);
                charts.temperature.update();
                console.log('✅ Successfully added test point to temperature chart');
            } catch (error) {
                console.error('❌ Failed to add test point:', error);
            }
        }
    };

    /**
     * Deep chart diagnosis function: logs each chart's canvas, datasets and an update
     * attempt, then Chart.js status. Only defined when {@link DEBUG_MODE} is on at load.
     * @returns {void}
     */
    window.diagnoseCharts = function() {
        console.log("🔬 DEEP CHART DIAGNOSIS:");
        
        Object.keys(charts).forEach(chartName => {
            const chart = charts[chartName];
            const canvas = document.getElementById(`${chartName}-chart`);
            
            console.log(`\n📊 ${chartName.toUpperCase()} CHART:`);
            console.log("  Chart Object:", !!chart);
            console.log("  Canvas Element:", !!canvas);
            console.log("  Canvas Visible:", canvas ? (canvas.offsetWidth > 0 && canvas.offsetHeight > 0) : false);
            console.log("  Canvas Dimensions:", canvas ? `${canvas.width}x${canvas.height}` : 'N/A');
            
            if (chart) {
                console.log("  Chart Data:");
                console.log("    Datasets:", chart.data?.datasets?.length || 0);
                console.log("    Labels:", chart.data?.labels?.length || 0);
                
                chart.data?.datasets?.forEach((dataset, idx) => {
                    console.log(`    Dataset ${idx}:`, {
                        label: dataset.label,
                        dataPoints: dataset.data?.length || 0,
                        lastPoint: dataset.data?.[dataset.data.length - 1],
                        borderColor: dataset.borderColor,
                        backgroundColor: dataset.backgroundColor
                    });
                });
                
                // Try to force update
                try {
                    chart.update('none');
                    console.log("  ✅ Chart.update() succeeded");
                } catch (error) {
                    console.log("  ❌ Chart.update() failed:", error);
                }
            }
        });
        
        // Also test Chart.js availability
        console.log("\n🔧 CHART.JS STATUS:");
        console.log("  Chart.js loaded:", typeof Chart !== 'undefined');
        console.log("  Chart version:", Chart?.version || 'Unknown');
        console.log("  Chart instances:", Chart?.instances?.length || 0);
    };
}

/**
 * Canvas inspection function: logs size, style and container details for each
 * dashboard chart canvas.
 * @returns {void}
 */
window.inspectCanvas = function() {
    console.log("🎨 CANVAS INSPECTION:");
    
    const chartNames = ['temperature', 'humidity', 'wind', 'rain', 'pressure', 'light', 'uv'];
    
    chartNames.forEach(chartName => {
        const canvas = document.getElementById(`${chartName}-chart`);
        const container = canvas?.parentElement;
        
        console.log(`\n🖼️ ${chartName.toUpperCase()} CANVAS:`);
        console.log("  Canvas exists:", !!canvas);
        
        if (canvas) {
            const rect = canvas.getBoundingClientRect();
            console.log("  Canvas position:", {
                width: canvas.width,
                height: canvas.height,
                clientWidth: canvas.clientWidth,
                clientHeight: canvas.clientHeight,
                offsetWidth: canvas.offsetWidth,
                offsetHeight: canvas.offsetHeight,
                boundingRect: {
                    width: rect.width,
                    height: rect.height,
                    top: rect.top,
                    left: rect.left
                }
            });
            
            console.log("  Canvas style:", {
                display: canvas.style.display || getComputedStyle(canvas).display,
                visibility: canvas.style.visibility || getComputedStyle(canvas).visibility,
                position: getComputedStyle(canvas).position
            });
            
            console.log("  Container:", {
                exists: !!container,
                className: container?.className,
                offsetWidth: container?.offsetWidth,
                offsetHeight: container?.offsetHeight
            });
        }
    });
};

/**
 * Force chart render function: calls `update('none')`, `render()` and `draw()` on
 * every chart and logs the result.
 * @returns {void}
 */
window.forceRenderCharts = function() {
    console.log("🔄 FORCING CHART RENDERS:");
    
    Object.keys(charts).forEach(chartName => {
        const chart = charts[chartName];
        if (chart) {
            try {
                console.log(`📊 Forcing render for ${chartName} chart...`);
                
                // Force multiple update strategies
                chart.update('none'); // No animation
                chart.render(); // Force immediate render
                chart.draw(); // Force draw
                
                console.log(`✅ ${chartName} chart render completed`);
                console.log(`   Data points: ${chart.data.datasets[0]?.data?.length || 0}`);
                console.log(`   Canvas attached: ${!!chart.canvas}`);
                console.log(`   Chart rendered: ${chart.rendered || 'unknown'}`);
            } catch (error) {
                console.error(`❌ Error rendering ${chartName} chart:`, error);
            }
        }
    });
};

/**
 * Recreate charts function: destroys every chart, clears {@link charts}, then after
 * 100 ms re-runs {@link initCharts} and, if weather data exists, {@link updateCharts}.
 * @returns {void}
 */
window.recreateCharts = function() {
    console.log("🔨 RECREATING ALL CHARTS:");
    
    // Destroy existing charts
    Object.keys(charts).forEach(chartName => {
        if (charts[chartName]) {
            try {
                charts[chartName].destroy();
                console.log(`🗑️ Destroyed ${chartName} chart`);
            } catch (error) {
                console.error(`Error destroying ${chartName} chart:`, error);
            }
        }
    });
    
    // Clear charts object
    Object.keys(charts).forEach(key => delete charts[key]);
    
    // Wait a moment then reinitialize
    setTimeout(() => {
        console.log("🔄 Reinitializing charts...");
        initCharts();
        
        // Try to repopulate with current data if available
        if (typeof weatherData !== 'undefined' && weatherData) {
            setTimeout(() => {
                updateCharts(weatherData);
                console.log("📊 Charts recreated and populated with current data");
            }, 100);
        }
    }, 100);
};

/**
 * Debug chart scales and force visible data: logs every dataset's values and every
 * scale's range, then appends three test points (20, 25, 30) to each chart's
 * dataset 0. Mutates live chart data.
 * @returns {void}
 */
window.debugChartScales = function() {
    console.log("🔍 DEBUGGING CHART SCALES:");
    
    Object.keys(charts).forEach(chartName => {
        const chart = charts[chartName];
        if (chart) {
            console.log(`\n📊 ${chartName.toUpperCase()} CHART SCALE DEBUG:`);
            
            // Check data ranges
            chart.data.datasets.forEach((dataset, idx) => {
                console.log(`  Dataset ${idx} (${dataset.label}):`);
                console.log(`    Data points: ${dataset.data.length}`);
                if (dataset.data.length > 0) {
                    const values = dataset.data.map(d => typeof d === 'object' ? d.y : d);
                    console.log(`    Values: [${values.join(', ')}]`);
                    console.log(`    Min: ${Math.min(...values)}, Max: ${Math.max(...values)}`);
                }
            });
            
            // Check scales
            if (chart.scales) {
                Object.values(chart.scales).forEach(scale => {
                    console.log(`  Scale ${scale.id}:`);
                    console.log(`    Type: ${scale.type}`);
                    console.log(`    Min: ${scale.min}, Max: ${scale.max}`);
                    console.log(`    Range: ${scale.max - scale.min}`);
                });
            }
            
            // Force a visible range and update
            try {
                // Add some test data points with visible values
                const testData = [
                    { x: new Date(Date.now() - 60000), y: 20 },
                    { x: new Date(Date.now() - 30000), y: 25 },
                    { x: new Date(), y: 30 }
                ];
                
                chart.data.datasets[0].data = [...chart.data.datasets[0].data, ...testData];
                chart.update('none');
                
                console.log(`  ✅ Added test data to ${chartName} chart`);
            } catch (error) {
                console.error(`  ❌ Error adding test data to ${chartName}:`, error);
            }
        }
    });
};

/**
 * Fix timestamp issues in all charts: replaces every dataset with three points at
 * now-2 min, now-1 min and now (90 %, 95 % and 100 % of its last value, or 20/25/30).
 * Destroys the real data.
 * @returns {void}
 */
window.fixChartTimestamps = function() {
    console.log("🔧 FIXING CHART TIMESTAMPS:");
    
    const now = new Date();
    const oneMinuteAgo = new Date(now.getTime() - 60000);
    const twoMinutesAgo = new Date(now.getTime() - 120000);
    
    Object.keys(charts).forEach(chartName => {
        const chart = charts[chartName];
        if (chart) {
            console.log(`📊 Fixing ${chartName} chart timestamps...`);
            
            // Clear existing data and add properly timestamped data
            chart.data.datasets.forEach((dataset, idx) => {
                // Get current values or use defaults
                let value1 = 20, value2 = 25, value3 = 30;
                
                if (dataset.data.length > 0) {
                    const lastPoint = dataset.data[dataset.data.length - 1];
                    const baseValue = typeof lastPoint === 'object' ? lastPoint.y : lastPoint;
                    value1 = baseValue * 0.9;
                    value2 = baseValue * 0.95; 
                    value3 = baseValue;
                }
                
                // Set properly timestamped data
                dataset.data = [
                    { x: twoMinutesAgo, y: value1 },
                    { x: oneMinuteAgo, y: value2 },
                    { x: now, y: value3 }
                ];
                
                console.log(`  Dataset ${idx}: ${dataset.data.length} points with proper timestamps`);
            });
            
            // Force chart update
            chart.update('none');
            console.log(`  ✅ ${chartName} chart timestamps fixed`);
        }
    });
    
    console.log("🎯 All chart timestamps fixed! Charts should now be visible.");
};

// ============================================
// Theme Switching System
// ============================================

/**
 * Load the saved theme on page load and keep it in sync with `#theme-select`,
 * persisting changes to localStorage (`selected-theme`).
 */
document.addEventListener('DOMContentLoaded', function() {
    const savedTheme = localStorage.getItem('selected-theme') || 'default';
    applyTheme(savedTheme);
    
    const themeSelect = document.getElementById('theme-select');
    if (themeSelect) {
        themeSelect.value = savedTheme;
        
        // Listen for theme changes
        themeSelect.addEventListener('change', function() {
            const newTheme = this.value;
            applyTheme(newTheme);
            localStorage.setItem('selected-theme', newTheme);
            debugLog(logLevels.INFO, `Theme changed to: ${newTheme}`);
        });
    }
});

/**
 * Apply a theme to the document via `body[data-theme]` and adjust chart colours.
 * @param {string} themeName Theme name; `'default'` removes the attribute, and
 *   `'midnight'` switches the charts to dark-mode colours.
 * @returns {void}
 */
function applyTheme(themeName) {
    const body = document.body;
    
    // Remove existing theme
    body.removeAttribute('data-theme');
    
    // Apply new theme (except for default)
    if (themeName !== 'default') {
        body.setAttribute('data-theme', themeName);
    }
    
    debugLog(logLevels.DEBUG, `Applied theme: ${themeName}`);
    
    // Update chart colors for dark themes
    if (themeName === 'midnight') {
        updateChartsForDarkMode(true);
    } else {
        updateChartsForDarkMode(false);
    }
}

/**
 * Update chart grid and tick colours for dark or light mode, then redraw.
 * @param {boolean} isDark `true` for the dark-theme colours.
 * @returns {void}
 */
function updateChartsForDarkMode(isDark) {
    const gridColor = isDark ? 'rgba(255, 255, 255, 0.1)' : 'rgba(0, 0, 0, 0.1)';
    const textColor = isDark ? '#e0e0e0' : '#666';
    
    Object.keys(charts).forEach(chartName => {
        const chart = charts[chartName];
        if (chart && chart.options) {
            // Update grid colors
            if (chart.options.scales && chart.options.scales.x && chart.options.scales.x.grid) {
                chart.options.scales.x.grid.color = gridColor;
            }
            if (chart.options.scales && chart.options.scales.y && chart.options.scales.y.grid) {
                chart.options.scales.y.grid.color = gridColor;
            }
            
            // Update tick colors
            if (chart.options.scales && chart.options.scales.x && chart.options.scales.x.ticks) {
                chart.options.scales.x.ticks.color = textColor;
            }
            if (chart.options.scales && chart.options.scales.y && chart.options.scales.y.ticks) {
                chart.options.scales.y.ticks.color = textColor;
            }
            
            chart.update('none');
        }
    });
}

/**
 * Fetch `/api/alarm-status` and display it via {@link updateAlarmStatus}.
 *
 * On any failure renders a placeholder status (disabled, "Error loading", no alarms)
 * instead of throwing.
 * @async
 * @returns {Promise<void>}
 */
async function fetchAlarmStatus() {
    try {
        debugLog(logLevels.DEBUG, 'Fetching alarm status...');
        const response = await fetch('/api/alarm-status');
        const data = await response.json();
        
        debugLog(logLevels.DEBUG, 'Alarm status received', data);
        updateAlarmStatus(data);
    } catch (error) {
        debugLog(logLevels.ERROR, 'Failed to fetch alarm status', error);
        updateAlarmStatus({
            enabled: false,
            configPath: 'Error loading',
            lastReadTime: 'N/A',
            totalAlarms: 0,
            enabledAlarms: 0,
            alarms: []
        });
    }
}

/**
 * Update the alarm status card and the list of enabled alarms.
 *
 * Callers (tests) may provide an explicit window/document via `opts`; otherwise
 * JSDOM-provided globals, then the real browser globals are used. Builds a tag
 * filter `<select>` from the alarms' tags (the `?tag=` URL parameter takes
 * precedence over the selection persisted in localStorage `alarm-selected-tag`;
 * changing it updates the URL and re-renders with the same data). Each enabled
 * alarm that matches the URL tag filter gets an expandable item (state persisted
 * in `alarm-expanded-set`) showing schedule state, condition, last trigger,
 * trigger count, channels, tags and cooldown. Honours `alarm-compact-mode`.
 * Exported for jest via `module.exports.updateAlarmStatus`.
 *
 * @param {AlarmStatus} data Parsed `/api/alarm-status` response.
 * @param {{window: (Window|undefined), document: (Document|undefined)}} [opts]
 *   Explicit globals to use instead of the ambient ones.
 * @returns {void}
 */
function updateAlarmStatus(data, opts) {
    // Allow callers (tests) to provide explicit window/document via opts
    const optWin = opts && opts.window ? opts.window : null;
    const optDoc = opts && opts.document ? opts.document : null;

    // Prefer explicit opts, then JSDOM-provided globals, then real browser globals
    const win = optWin || ((typeof global !== 'undefined' && global.window) ? global.window : (typeof window !== 'undefined' ? window : null));
    const doc = optDoc || ((win && win.document) ? win.document : ((typeof global !== 'undefined' && global.document) ? global.document : (typeof document !== 'undefined' ? document : null)));
    if (!doc) return; // cannot operate without a document

    // debug: win presence logged during development
    const storage = win && win.localStorage ? win.localStorage : null;

    // Update status indicator
    const statusEl = doc.getElementById('alarm-status');
    if (statusEl) {
        if (data.enabled) {
            statusEl.textContent = '✅ Active';
            statusEl.style.color = 'var(--success-color, #4caf50)';
        } else if (data.disabled) {
            statusEl.textContent = '❌ Disabled';
            statusEl.style.color = 'var(--error-color, #dc3545)';
        } else {
            statusEl.textContent = '⚠️ Not Configured';
            statusEl.style.color = 'var(--warning-color, #ff9800)';
        }
    }
    
    // Update config path
    const configPathEl = doc.getElementById('alarm-config-path');
    if (configPathEl) {
        configPathEl.textContent = data.configPath || 'N/A';
    }
    
    // Update last read time
    const lastReadEl = doc.getElementById('alarm-last-read');
    if (lastReadEl) {
        lastReadEl.textContent = data.lastReadTime || 'N/A';
    }
    
    // Update counts
    const totalCountEl = doc.getElementById('alarm-total-count');
    if (totalCountEl) {
        totalCountEl.textContent = data.totalAlarms || 0;
    }
    
    const enabledCountEl = doc.getElementById('alarm-enabled-count');
    if (enabledCountEl) {
        enabledCountEl.textContent = data.enabledAlarms || 0;
    }
    
    // Update alarm list
    const alarmListEl = doc.getElementById('alarm-list');
    if (alarmListEl && data.alarms && data.alarms.length > 0) {
        // Clear existing items except header
        const header = alarmListEl.querySelector('.alarm-list-header');
        alarmListEl.innerHTML = '';
        if (header) {
            alarmListEl.appendChild(header);
        } else {
            alarmListEl.innerHTML = '<div class="alarm-list-header">Active Alarms:</div>';
        }
        
        // Check for tag filter in URL (e.g. ?tag=outdoor)
        const urlParamsLocal = new URLSearchParams((win && win.location) ? win.location.search : '');
        const filterTag = urlParamsLocal.get('tag');

        // Update header to indicate if filtered by tag and add tag selector
        const headerEl = alarmListEl.querySelector('.alarm-list-header');
        if (headerEl) {
            // Create or reuse a container for header controls
            let headerControls = headerEl.querySelector('.alarm-header-controls');
            if (!headerControls) {
                headerControls = doc.createElement('div');
                headerControls.className = 'alarm-header-controls';
                headerControls.style.display = 'inline-block';
                headerControls.style.marginLeft = '12px';
                headerEl.appendChild(headerControls);
            }

            // Always show the main header text; tag is shown as a control in parentheses
            headerEl.firstChild && (headerEl.firstChild.textContent = 'Active Alarms');

            // Build unique tag list from data.alarms
            const tagSet = new Set();
            data.alarms.forEach(a => {
                if (Array.isArray(a.tags)) {
                    a.tags.forEach(t => tagSet.add(t));
                }
            });

            // Determine selected tag (URL param takes precedence)
            const persistedTag = (() => { try { return (win && win.localStorage) ? win.localStorage.getItem('alarm-selected-tag') : (typeof localStorage !== 'undefined' ? localStorage.getItem('alarm-selected-tag') : null); } catch(e) { return null; } })();
            let selectedTag = filterTag || '';
            if (!selectedTag && persistedTag && tagSet.has(persistedTag)) {
                selectedTag = persistedTag;
                // Update URL to reflect persisted selection
                const params = new URLSearchParams((win && win.location) ? win.location.search : '');
                params.set('tag', selectedTag);
                const newUrl = ((win && win.location) ? win.location.pathname : '/') + (params.toString() ? ('?' + params.toString()) : '');
                if (win && win.history && typeof win.history.replaceState === 'function') win.history.replaceState({}, '', newUrl);
            }

            // Create dropdown if not present
            let tagSelect = headerControls.querySelector('select.alarm-tag-select');
            if (!tagSelect) {
                const label = doc.createElement('label');
                label.textContent = ' (tag: ';
                label.htmlFor = 'alarm-tag-select';
                label.style.fontSize = '0.9em';
                label.style.marginRight = '4px';

                tagSelect = doc.createElement('select');
                tagSelect.id = 'alarm-tag-select';
                tagSelect.className = 'alarm-tag-select';
                tagSelect.setAttribute('aria-label', 'Filter alarms by tag');
                tagSelect.style.fontSize = '0.9em';
                tagSelect.style.padding = '2px 6px';

                const closing = doc.createElement('span');
                closing.textContent = ')';
                closing.style.marginLeft = '4px';

                headerControls.appendChild(label);
                headerControls.appendChild(tagSelect);
                headerControls.appendChild(closing);

                // Attach change handler to update URL and refresh
                tagSelect.addEventListener('change', function() {
                    const val = this.value;
                    const params = new URLSearchParams((win && win.location) ? win.location.search : '');
                    if (val === '') {
                        params.delete('tag');
                        try { if (win && win.localStorage) win.localStorage.removeItem('alarm-selected-tag'); else localStorage.removeItem('alarm-selected-tag'); } catch(e){}
                    } else {
                        params.set('tag', val);
                        try { if (win && win.localStorage) win.localStorage.setItem('alarm-selected-tag', val); else localStorage.setItem('alarm-selected-tag', val); } catch(e){}
                    }
                    const newUrl = ((win && win.location) ? win.location.pathname : '/') + (params.toString() ? ('?' + params.toString()) : '');
                    if (win && win.history && typeof win.history.replaceState === 'function') win.history.replaceState({}, '', newUrl);
                    // Re-render using current data (no need to fetch again)
                    updateAlarmStatus(data, { window: win, document: doc });
                });
            }

            // Populate the select with options
            // Preserve current selection if still valid
            tagSelect.innerHTML = '';
            const emptyOpt = doc.createElement('option');
            emptyOpt.value = '';
            emptyOpt.textContent = '-- all tags --';
            tagSelect.appendChild(emptyOpt);
            Array.from(tagSet).sort().forEach(t => {
                const opt = doc.createElement('option');
                opt.value = t;
                opt.textContent = t;
                if (t === selectedTag) opt.selected = true;
                tagSelect.appendChild(opt);
            });
            // Ensure selection reflects selectedTag
            tagSelect.value = selectedTag;
        }

        // Add alarm items
        data.alarms.forEach(alarm => {
            if (!alarm.enabled) return; // Only show enabled alarms
            if (filterTag && Array.isArray(alarm.tags) && alarm.tags.indexOf(filterTag) === -1) return; // Apply tag filter
            
            const alarmItem = doc.createElement('div');
            alarmItem.className = 'alarm-item';
            // Respect compact mode preference stored in localStorage
            const globalStorageValue = (typeof localStorage !== 'undefined' && localStorage) ? localStorage.getItem('alarm-compact-mode') : null;
            // localStorage debug removed
            const isAlarmCompactLocal = (storage && (storage.getItem('alarm-compact-mode') === 'true')) || (globalStorageValue === 'true');
            if (isAlarmCompactLocal) {
                alarmItem.classList.add('compact');
                // compact mode applied
            }
            
            // Check for expanded state (disclosure triangles control visibility)
            const realStorage = storage || (typeof localStorage !== 'undefined' ? localStorage : null);
            const expandedSet = realStorage ? realStorage.getItem('alarm-expanded-set') : null;
            let expandedKeys = [];
            if (expandedSet) {
                try {
                    expandedKeys = JSON.parse(expandedSet);
                } catch(e) {}
            }
            if (expandedKeys.includes(alarm.name)) {
                alarmItem.classList.add('expanded');
            }
            
            const alarmName = doc.createElement('div');
            alarmName.className = 'alarm-item-name';
            
            // Add expand/collapse button
            const expandButton = doc.createElement('button');
            expandButton.className = 'alarm-expand-button';
            expandButton.textContent = alarmItem.classList.contains('expanded') ? '▼' : '▶';
            expandButton.setAttribute('aria-label', 'Toggle alarm details');
            expandButton.addEventListener('click', function(e) {
                e.stopPropagation();
                alarmItem.classList.toggle('expanded');
                this.textContent = alarmItem.classList.contains('expanded') ? '▼' : '▶';
                // Update localStorage
                const expandedSet = realStorage ? realStorage.getItem('alarm-expanded-set') : null;
                let expandedKeys = [];
                if (expandedSet) {
                    try {
                        expandedKeys = JSON.parse(expandedSet);
                    } catch(e) {}
                }
                const name = alarm.name;
                if (alarmItem.classList.contains('expanded')) {
                    if (!expandedKeys.includes(name)) {
                        expandedKeys.push(name);
                    }
                } else {
                    expandedKeys = expandedKeys.filter(k => k !== name);
                }
                try {
                    if (realStorage) realStorage.setItem('alarm-expanded-set', JSON.stringify(expandedKeys));
                } catch(e) {}
            });
            
            alarmName.appendChild(expandButton);
            alarmName.appendChild(doc.createTextNode(` 🔔 ${alarm.name}`));
            
            // Add schedule icon if alarm has a schedule
            if (alarm.hasSchedule) {
                const scheduleIcon = doc.createElement('span');
                scheduleIcon.className = 'alarm-schedule-icon';
                scheduleIcon.style.marginLeft = '8px';
                scheduleIcon.style.fontSize = '0.9em';
                if (alarm.scheduleActive) {
                    scheduleIcon.textContent = '🕐✅';
                    scheduleIcon.title = 'Scheduled alarm - Currently ACTIVE';
                    scheduleIcon.style.color = 'var(--success-color, #4caf50)';
                } else {
                    scheduleIcon.textContent = '🕐⏸️';
                    scheduleIcon.title = 'Scheduled alarm - Currently INACTIVE (outside schedule)';
                    scheduleIcon.style.color = 'var(--warning-color, #ff9800)';
                }
                alarmName.appendChild(scheduleIcon);
            }
            
            const alarmDetails = doc.createElement('div');
            alarmDetails.className = 'alarm-item-details';
            
            const condition = doc.createElement('div');
            condition.className = 'alarm-item-condition';
            condition.textContent = `Condition: ${alarm.condition}`;
            
            const lastTriggered = doc.createElement('div');
            lastTriggered.className = 'alarm-item-triggered';
            lastTriggered.textContent = `Last: ${alarm.lastTriggered}`;
            
            const channels = doc.createElement('div');
            channels.className = 'alarm-item-channels';
            channels.textContent = `Channels: ${Array.isArray(alarm.channels) ? alarm.channels.join(', ') : '-'}`;

            // Tags display
            const tagsEl = doc.createElement('div');
            tagsEl.className = 'alarm-item-tags';
            if (Array.isArray(alarm.tags) && alarm.tags.length > 0) {
                tagsEl.textContent = `Tags: ${alarm.tags.join(', ')}`;
            } else {
                tagsEl.textContent = 'Tags: -';
            }
            
            // Add cooldown status if applicable
            const cooldown = doc.createElement('div');
            cooldown.className = 'alarm-item-cooldown';
            if (alarm.inCooldown) {
                const minutes = Math.floor(alarm.cooldownRemaining / 60);
                const seconds = alarm.cooldownRemaining % 60;
                const timeStr = minutes > 0 ? `${minutes}m ${seconds}s` : `${seconds}s`;
                cooldown.textContent = `⏳ Cooldown: ${timeStr} remaining`;
                cooldown.style.color = 'var(--warning-color, #ff9800)';
            } else {
                cooldown.textContent = `✓ Ready (cooldown: ${alarm.cooldown}s)`;
                cooldown.style.color = 'var(--success-color, #4caf50)';
            }
            
            alarmDetails.appendChild(condition);
            alarmDetails.appendChild(lastTriggered);

            // Triggered count badge
            const triggeredCountEl = doc.createElement('div');
            triggeredCountEl.className = 'alarm-item-triggered-count';
            // API provides triggeredCount (number of times alarm fired during process lifetime)
            const triggeredCount = Number(alarm.triggeredCount || 0);
            triggeredCountEl.textContent = `Triggered: ${triggeredCount}`;
            // Add a visual warning class when the count is > 0
            if (triggeredCount > 5) {
                triggeredCountEl.classList.add('alarm-triggered-critical');
            } else if (triggeredCount > 0) {
                triggeredCountEl.classList.add('alarm-triggered-warn');
            } else {
                triggeredCountEl.classList.add('alarm-triggered-ok');
            }

            alarmDetails.appendChild(triggeredCountEl);
            alarmDetails.appendChild(channels);
            alarmDetails.appendChild(tagsEl);
            alarmDetails.appendChild(cooldown);
            
            alarmItem.appendChild(alarmName);
            alarmItem.appendChild(alarmDetails);
            
            alarmListEl.appendChild(alarmItem);
        });
    } else if (alarmListEl) {
        alarmListEl.innerHTML = '<div class="alarm-list-header">Active Alarms:</div><div class="alarm-list-empty">No active alarms configured</div>';
    }
}

// Ensure updateAlarmStatus is available on window (browser) and exportable for tests
try {
    if (typeof window !== 'undefined' && window) {
        // Attach only if not already present
        window.updateAlarmStatus = window.updateAlarmStatus || updateAlarmStatus;
    }
} catch (e) {
    // ignore
}

// CommonJS export for Node/Jest tests
if (typeof module !== 'undefined' && module.exports) {
    module.exports = module.exports || {};
    module.exports.updateAlarmStatus = updateAlarmStatus;
}