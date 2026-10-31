# frozen_string_literal: true

# Date and time value represented as a Unix timestamp.
#
# DateTime uses the same UTC calendar model as AIS::Time and shares its
# calendar calculations through AISCalendar. It is intentionally independent
# of the host operating system's timezone and does not require Ruby's `date`
# standard library.
#
# Supported parsing formats:
#
# - `YYYY-MM-DD`
# - `YYYY-MM-DDTHH:MM:SSZ`
# - `YYYY-MM-DDTHH:MM:SS+HH:MM`
# - `YYYY-MM-DDTHH:MM:SS-HH:MM`
#
# Parsed offsets are normalized to UTC. Fractional seconds and named timezone
# identifiers are not supported.
#
# @example Current date and time
#   now = DateTime.now
#   now.year
#   now.month
#   now.day
#
# @example Parse an ISO-8601 date
#   DateTime.parse('2026-10-31')
#
# @example Parse a timestamp with an offset
#   DateTime.parse('2026-10-31T12:30:00+05:30')
#
# @see Time
class DateTime
  include Comparable

  # Unix timestamp in whole seconds.
  # @return [Integer]
  attr_reader :epoch

  # Returns the current request time supplied by the AIS gateway.
  # @return [DateTime]
  def self.now
    DateTime.at(AIS.current_time_epoch)
  end

  # Returns midnight UTC at the start of the current UTC date.
  # @return [DateTime]
  def self.today
    current = DateTime.now
    DateTime.new(AISCalendar.days_from_civil(current.year, current.month, current.day) *
                 AISCalendar::SECONDS_PER_DAY)
  end

  # Constructs a DateTime from Unix seconds.
  #
  # @param epoch [Integer, Numeric] seconds since the Unix epoch
  # @return [DateTime]
  def self.at(epoch)
    DateTime.new(epoch)
  end

  # Parses a supported ISO-8601 date or date-time string.
  #
  # Date-only strings are interpreted as midnight UTC. Date-times without an
  # explicit offset are also interpreted as UTC. Explicit numeric offsets are
  # converted to UTC.
  #
  # @param value [String] ISO-8601 date or date-time
  # @return [DateTime]
  # @raise [ArgumentError] if the string is malformed or contains an invalid date
  def self.parse(value)
    unless value.is_a?(String)
      raise ArgumentError, 'DateTime.parse expects a String'
    end

    match = /\A(\d{4})-(\d{2})-(\d{2})(?:T(\d{2}):(\d{2}):(\d{2})(Z|[+-]\d{2}:\d{2})?)?\z/.match(value)
    raise ArgumentError, "invalid date-time: #{value}" unless match

    year = match[1].to_i
    month = match[2].to_i
    day = match[3].to_i
    hour = match[4] ? match[4].to_i : 0
    minute = match[5] ? match[5].to_i : 0
    second = match[6] ? match[6].to_i : 0
    offset = match[7]

    unless AISCalendar.valid_date?(year, month, day)
      raise ArgumentError, "invalid calendar date: #{value}"
    end
    if hour > 23 || minute > 59 || second > 59
      raise ArgumentError, "invalid clock time: #{value}"
    end

    offset_seconds = 0
    if offset && offset != 'Z'
      offset_hours = offset[1, 2].to_i
      offset_minutes = offset[4, 2].to_i
      if offset_hours > 23 || offset_minutes > 59
        raise ArgumentError, "invalid UTC offset: #{value}"
      end
      offset_seconds = (offset_hours * 3_600 + offset_minutes * 60)
      offset_seconds = -offset_seconds if offset[0, 1] == '-'
    end

    local_epoch = AISCalendar.days_from_civil(year, month, day) *
                  AISCalendar::SECONDS_PER_DAY +
                  hour * 3_600 + minute * 60 + second

    # A positive textual offset means local time is ahead of UTC.
    DateTime.at(local_epoch - offset_seconds)
  end

  # @param epoch [Integer, Numeric] Unix timestamp in seconds
  def initialize(epoch)
    @epoch = epoch.to_i
  end

  # Returns the Gregorian year in UTC.
  # @return [Integer]
  def year
    calendar_parts[0]
  end

  # Returns the month number in UTC (January is 1).
  # @return [Integer]
  def month
    calendar_parts[1]
  end

  # Returns the day of the month in UTC (1..31).
  # @return [Integer]
  def day
    calendar_parts[2]
  end

  # Returns the day of the year in UTC (January 1 is 1).
  # @return [Integer]
  def yday
    @epoch.div(AISCalendar::SECONDS_PER_DAY) -
      AISCalendar.days_from_civil(year, 1, 1) + 1
  end

  # Returns the weekday in UTC. Sunday is 0 and Saturday is 6.
  # @return [Integer]
  def wday
    (@epoch.div(AISCalendar::SECONDS_PER_DAY) + 4) % 7
  end

  # Returns the hour in UTC (0..23).
  # @return [Integer]
  def hour
    seconds_of_day / 3_600
  end

  # Returns the minute in UTC (0..59).
  # @return [Integer]
  def minute
    seconds_of_day % 3_600 / 60
  end

  # Returns the second in UTC (0..59).
  # @return [Integer]
  def second
    seconds_of_day % 60
  end

  # Adds a duration.
  # @param duration [AISDuration, Numeric] duration in seconds
  # @return [DateTime]
  def +(duration)
    DateTime.at(@epoch + duration.to_i)
  end

  # Subtracts another DateTime or a duration.
  # @param other [DateTime, AISDuration, Numeric]
  # @return [Integer, DateTime]
  def -(other)
    if other.is_a?(DateTime)
      @epoch - other.to_i
    else
      DateTime.at(@epoch - other.to_i)
    end
  end

  # Compares two DateTime values.
  # @param other [DateTime]
  # @return [Integer, nil]
  def <=>(other)
    return nil unless other.is_a?(DateTime)

    @epoch <=> other.to_i
  end

  # Tests timestamp equality.
  # @param other [Object]
  # @return [Boolean]
  def ==(other)
    other.is_a?(DateTime) && @epoch == other.to_i
  end

  # Returns the Unix timestamp.
  # @return [Integer]
  def to_i
    @epoch
  end

  # Returns the equivalent AIS Time instance.
  # @return [Time]
  def to_time
    Time.at(@epoch)
  end

  # Returns an ISO-8601 UTC representation.
  # @return [String] YYYY-MM-DDTHH:MM:SSZ
  def to_s
    sprintf('%04d-%02d-%02dT%02d:%02d:%02dZ',
            year, month, day, hour, minute, second)
  end

  # Returns the date portion in ISO-8601 form.
  # @return [String] YYYY-MM-DD
  def to_date_s
    sprintf('%04d-%02d-%02d', year, month, day)
  end

  private

  def calendar_parts
    AISCalendar.civil_from_days(@epoch.div(AISCalendar::SECONDS_PER_DAY))
  end

  def seconds_of_day
    @epoch - @epoch.div(AISCalendar::SECONDS_PER_DAY) *
             AISCalendar::SECONDS_PER_DAY
  end
end
