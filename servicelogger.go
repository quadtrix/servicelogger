// Package servicelogger - implements a file logger for services
package servicelogger

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
)

type LogLevel int

const (
	LL_TRACE LogLevel = 1
	LL_DEBUG LogLevel = 2
	LL_INFO  LogLevel = 3
	LL_WARN  LogLevel = 4
	LL_ERROR LogLevel = 5
	LL_FATAL LogLevel = 6
)

type Logger struct {
	base             *log.Logger
	prefix           string
	MinLoglevel      LogLevel
	filename         string
	rotate           bool
	rotatesize       int64
	keep             int
	filehandle       *os.File
	rotation_running bool
	filters          FacilityFilters
}

type FacilityFilter struct {
	filter string
	level  LogLevel
}

type FacilityFilters struct {
	count   int
	filters []FacilityFilter
}

func logSizeStringToLogSizeInt64(lss string) (l int64, err error) {
	modifier := 0
	if strings.HasSuffix(lss, "K") {
		modifier = 1024
	} else if strings.HasSuffix(lss, "M") {
		modifier = 1024 * 1024
	} else if strings.HasSuffix(lss, "G") {
		modifier = 1024 * 1024 * 1024
	} else if strings.HasSuffix(lss, "T") {
		modifier = 1024 * 1024 * 1024 * 1024
	} else {
		return l, errors.New("unknown rotation size modifier, allowed modifiers: 'K', 'M', 'G', 'T'")
	}
	il, err := strconv.Atoi(lss[:len(lss)-1])
	if err != nil {
		return l, err
	}
	l = int64(il * modifier)
	return l, nil
}

// New returns a new Logger object
func New(prefix string, filename string, minloglevel LogLevel, rotate bool, rotatesize string, keep int) (l Logger, err error) {
	l.filename = filename
	l.rotate = rotate
	if keep < 2 && rotate {
		return l, errors.New("keep_rotated too low (>=2)")
	}
	l.keep = keep
	l.prefix = prefix
	l.rotatesize, err = logSizeStringToLogSizeInt64(rotatesize)
	if err != nil {
		log.Fatal(fmt.Sprintf("FATAL: Incorrect log rotation size: %s", err.Error()))
	}
	l.filehandle, err = os.OpenFile(filename, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0640)
	if err != nil {
		log.Fatal("FATAL: Unable to open log file: " + err.Error())
	}

	l.MinLoglevel = minloglevel
	l.base = log.New(l.filehandle, "", log.Ldate|log.Ltime|log.Lmicroseconds)
	l.rotation_running = false
	return l, err
}

// LogTrace logs a message at TRACE level
func (l Logger) LogTrace(function string, source string, text string) {
	if l.getFilteredLogLevel(fmt.Sprintf("%s.%s.%s", l.prefix, source, function)) <= LL_TRACE {
		l.logRotate()
		l.base.Println(fmt.Sprintf("TRACE   [%s] %s.%s %s", function, l.prefix, source, text))
	}
}

// LogDebug logs a message at DEBUG level
func (l Logger) LogDebug(function string, source string, text string) {
	if l.getFilteredLogLevel(fmt.Sprintf("%s.%s.%s", l.prefix, source, function)) <= LL_DEBUG {
		l.logRotate()
		l.base.Println(fmt.Sprintf("DEBUG   [%s] %s.%s %s", function, l.prefix, source, text))
	}
}

// LogInfo logs a message at INFO level
func (l Logger) LogInfo(function string, source string, text string) {
	if l.getFilteredLogLevel(fmt.Sprintf("%s.%s.%s", l.prefix, source, function)) <= LL_INFO {
		l.logRotate()
		l.base.Println(fmt.Sprintf("INFO    [%s] %s.%s %s", function, l.prefix, source, text))
	}
}

// LogWarn logs a message at WARNING level
func (l Logger) LogWarn(function string, source string, text string) {
	if l.getFilteredLogLevel(fmt.Sprintf("%s.%s.%s", l.prefix, source, function)) <= LL_WARN {
		l.logRotate()
		l.base.Println(fmt.Sprintf("WARNING [%s] %s.%s %s", function, l.prefix, source, text))
	}
}

// LogError logs a message at ERROR level
func (l Logger) LogError(function string, source string, text string) {
	if l.getFilteredLogLevel(fmt.Sprintf("%s.%s.%s", l.prefix, source, function)) <= LL_ERROR {
		l.logRotate()
		l.base.Println(fmt.Sprintf("ERROR   [%s] %s.%s %s", function, l.prefix, source, text))
	}
}

// LogFata logs a message at FATAL level and exits the application with the provided exit code
func (l Logger) LogFatal(function string, source string, text string, exitcode int) {
	if l.getFilteredLogLevel(fmt.Sprintf("%s.%s.%s", l.prefix, source, function)) <= LL_FATAL {
		l.logRotate()
		l.base.Println(fmt.Sprintf("FATAL   [%s] %s.%s %s", function, l.prefix, source, text))
		fmt.Println(fmt.Sprintf("FATAL: [%s] %s.%s %s", function, l.prefix, source, text))
		os.Exit(exitcode)
	}
}

// StringToLogLevel returns a LogLevel for a provided string. When the string cannot be recognised, LL_INFO is returned
func StringToLogLevel(text string) LogLevel {
	switch text {
	case "TRACE", "Trace", "trace":
		return LL_TRACE
	case "DEBUG", "Debug", "debug":
		return LL_DEBUG
	case "INFO", "Info", "info":
		return LL_INFO
	case "WARN", "Warn", "warn":
		return LL_WARN
	case "ERROR", "Error", "error":
		return LL_ERROR
	case "FATAL", "Fatal", "fatal":
		return LL_FATAL
	default:
		return LL_INFO
	}
}

// LogLevelToString returns a string representation of the LogLevel
func LogLevelToString(level LogLevel) string {
	switch level {
	case LL_TRACE:
		return "TRACE"
	case LL_DEBUG:
		return "DEBUG"
	case LL_INFO:
		return "INFO"
	case LL_WARN:
		return "WARN"
	case LL_ERROR:
		return "ERROR"
	case LL_FATAL:
		return "FATAL"
	default:
		return "UNKNOWN"
	}
}

func (l *Logger) logRotate() (err error) {
	if l.rotate && !l.rotation_running {
		l.rotation_running = true
		//l.LogTrace("logRotate", "servicelogger", "Starting log rotation check")
		filestats, err := os.Stat(l.filename)
		if err != nil {
			return err
		}
		if filestats.Size() >= l.rotatesize {
			l.LogTrace("logRotate", "servicelogger", "Rotating log, closing logwriter")
			_, err = os.Stat(fmt.Sprintf("%s.%d", l.filename, l.keep))
			if err == nil {
				err = os.Remove(fmt.Sprintf("%s.%d", l.filename, l.keep))
			}
			for i := l.keep - 1; i > 0; i-- {
				_, err = os.Stat(fmt.Sprintf("%s.%d", l.filename, i))
				if err == nil {
					err = os.Rename(fmt.Sprintf("%s.%d", l.filename, i), fmt.Sprintf("%s.%d", l.filename, i+1))
					if err != nil {
						return err
					}
				}
			}
			err = os.Rename(l.filename, fmt.Sprintf("%s.1", l.filename))
			if err != nil {
				return err
			}
			l.filehandle, err = os.OpenFile(l.filename, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0640)
			if err != nil {
				log.Fatal(fmt.Sprintf("FATAL: Unable to open log file %s: %s", l.filename, err.Error()))
			}
			l.base = log.New(l.filehandle, "", log.Ldate|log.Ltime|log.Lmicroseconds)
			l.LogTrace("logRotate", "servicelogger", "Log rotated, reopened logwriter")
			return nil
		}
		l.rotation_running = false
	}
	return nil
}

func (slog *Logger) ApplyNewSettings(newFile string, newLevel LogLevel, newRotation bool, newRotSize string, newKeep int) bool {
	nrs, err := logSizeStringToLogSizeInt64(newRotSize)
	if err != nil {
		slog.LogFatal("ApplyNewSettings", "servicelogger", fmt.Sprintf("Failed to apply new settings: %s", err.Error()), 222)
	}
	if newFile != slog.filename || newLevel != slog.MinLoglevel || newRotation != slog.rotate || nrs != slog.rotatesize || newKeep != slog.keep {
		slog.LogInfo("ApplyNewSettings", "servicelogger", "Logging configuration has changed, applying new configuration")
		if newFile != slog.filename {
			slog.LogTrace("ApplyNewSettings", "servicelogger", fmt.Sprintf("Filename has changed. Closing %s and continuing logging in %s", slog.filename, newFile))
			slog.filehandle.Close()
			slog.filename = newFile
			slog.filehandle, err = os.OpenFile(slog.filename, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0640)
			if err != nil {
				log.Fatal("FATAL: Unable to open log file: " + err.Error())
			}
			slog.base = log.New(slog.filehandle, "", log.Ldate|log.Ltime|log.Lmicroseconds)
		}
		if newLevel != slog.MinLoglevel {
			slog.LogTrace("ApplyNewSettings", "servicelogger", fmt.Sprintf("Log level has changed: %s --> %s", LogLevelToString(slog.MinLoglevel), LogLevelToString(newLevel)))
			slog.MinLoglevel = newLevel
		}
		if newRotation != slog.rotate {
			if newRotation {
				slog.LogTrace("ApplyNewSettings", "servicelogger", "Log rotation has been enabled")
			} else {
				slog.LogTrace("ApplyNewSettings", "servicelogger", "Log rotation has been disabled")
			}
			slog.rotate = newRotation
		}
		if nrs != slog.rotatesize {
			slog.LogTrace("ApplyNewSettings", "servicelogger", fmt.Sprintf("Log size limit has changed: %d bytes --> %d bytes", slog.rotatesize, nrs))
			slog.rotatesize = nrs
		}
		if newKeep != slog.keep {
			slog.LogTrace("ApplyNewSettings", "servicelogger", fmt.Sprintf("Number of log files to keep has changed: %d --> %d", slog.keep, newKeep))
			slog.keep = newKeep
		}
		return true
	}
	return false
}

func (slog *Logger) AddFacilityFilter(filtername string, filterlevel LogLevel) {
	ffilter := FacilityFilter{
		filter: filtername,
		level:  filterlevel,
	}
	slog.filters.count++
	slog.filters.filters = append(slog.filters.filters, ffilter)
}

func (slog *Logger) LoadFacilityFilters(filename string) error {
	fcontents, err := os.ReadFile(filename)
	if err != nil {
		return err
	}
	var lfilters map[string]string
	err = json.Unmarshal(fcontents, &lfilters)
	if err != nil {
		return err
	}
	for fname, flevel := range lfilters {
		lflevel := StringToLogLevel(flevel)
		slog.AddFacilityFilter(fname, lflevel)
	}
	return nil
}

func (slog Logger) getFilteredLogLevel(facility string) LogLevel {
	fmt.Println(fmt.Sprintf("Determining filtered level for facility %s", facility))
	var foundfilter int = -1
	for n, filter := range slog.filters.filters {
		if strings.HasPrefix(facility, filter.filter) {
			fmt.Println(fmt.Sprintf("Filter %s matches", filter.filter))
			if foundfilter > -1 {
				if len(filter.filter) > len(slog.filters.filters[foundfilter].filter) {
					foundfilter = n
					fmt.Println("This is now the best match")
				}
			}
		}
	}
	if foundfilter > -1 {
		fmt.Println(fmt.Sprintf("After checking filters, the best match is %s, with log level %s", slog.filters.filters[foundfilter].filter, LogLevelToString(slog.filters.filters[foundfilter].level)))
		return slog.filters.filters[foundfilter].level
	}
	fmt.Println("No match was found, returning the default log level")
	return slog.MinLoglevel
}

func (slog Logger) DumpLogFilters() FacilityFilters {
	return slog.filters
}
