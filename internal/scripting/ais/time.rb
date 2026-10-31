# frozen_string_literal: true

# Calendar calculations shared by AIS::Time and AIS::DateTime.
#
# This module deliberately avoids Ruby's platform Time implementation and
# external date libraries so that the API remains usable in the embedded mruby
# runtime. All calendar components are UTC unless explicitly stated otherwise.
#
# @api private
module AISCalendar
  SECONDS_PER_DAY = 86_400

  def self.leap_year?(year)
    (year % 4 == 0 && year % 100 != 0) || year % 400 == 0
  end

  def self.days_in_month(year, month)
    return nil unless month >= 1 && month <= 12

    lengths = [31, leap_year?(year) ? 29 : 28, 31, 30, 31, 30,
               31, 31, 30, 31, 30, 31]
    lengths[month - 1]
  end

  # Convert a Gregorian civil date to days since 1970-01-01.
  def self.days_from_civil(year, month, day)
    year -= 1 if month <= 2
    era = year.div(400)
    year_of_era = year - era * 400
    adjusted_month = month + (month > 2 ? -3 : 9)
    day_of_year = (153 * adjusted_month + 2).div(5) + day - 1
    day_of_era = year_of_era * 365 + year_of_era.div(4) -
                 year_of_era.div(100) + day_of_year
    era * 146_097 + day_of_era - 719_468
  end

  # Convert days since 1970-01-01 to [year, month, day].
  def self.civil_from_days(days)
    shifted = days + 719_468
    era = shifted.div(146_097)
    day_of_era = shifted - era * 146_097
    year_of_era = (day_of_era - day_of_era.div(1_460) +
                   day_of_era.div(36_524) - day_of_era.div(146_096)).div(365)
    year = year_of_era + era * 400
    day_of_year = day_of_era -
                  (365 * year_of_era + year_of_era.div(4) - year_of_era.div(100))
    month_part = (5 * day_of_year + 2).div(153)
    day = day_of_year - (153 * month_part + 2).div(5) + 1
    month = month_part + (month_part < 10 ? 3 : -9)
    year += 1 if month <= 2
    [year, month, day]
  end

  def self.valid_date?(year, month, day)
    maximum = days_in_month(year, month)
    !maximum.nil? && day >= 1 && day <= maximum
  end
end

# Lightweight UTC timestamp used by AIS policies.
#
# `Time` intentionally provides a small, deterministic subset of Ruby's
# standard Time API. It stores Unix seconds and does not consult the host
# machine's timezone. Use {DateTime} for date-oriented construction and parsing.
#
# @example Read the current UTC time
#   now = Time.now
#   now.hour
#   now.to_s
#
# @example Perform duration arithmetic
#   Time.now + 2.hours
#   30.minutes.ago
#
# @see DateTime
class Time
  include Comparable

  # Unix timestamp in whole seconds.
  #
  # @return [Integer]
  attr_reader :epoch

  # Returns the request's current timestamp supplied by the AIS gateway.
  #
  # The value is request-scoped, making policy evaluation deterministic for
  # the lifetime of a request.
  #
  # @return [Time] current UTC time
  # @example
  #   Time.now
  def self.now
    Time.at(AIS.current_time_epoch)
  end

  # Constructs a time from Unix seconds.
  #
  # @param epoch [Integer, Numeric] seconds since 1970-01-01 00:00:00 UTC
  # @return [Time]
  # @example
  #   Time.at(1_793_424_000)
  def self.at(epoch)
    Time.new(epoch)
  end

  # @param epoch [Integer, Numeric] Unix timestamp in seconds
  def initialize(epoch)
    @epoch = epoch.to_i
  end

  # Returns the Gregorian year in UTC.
  # @return [Integer]
  def year
    AISCalendar.civil_from_days(@epoch.div(AISCalendar::SECONDS_PER_DAY))[0]
  end

  # Returns the month number in UTC (January is 1).
  # @return [Integer]
  def month
    AISCalendar.civil_from_days(@epoch.div(AISCalendar::SECONDS_PER_DAY))[1]
  end

  # Returns the day of the month in UTC (1..31).
  # @return [Integer]
  def day
    AISCalendar.civil_from_days(@epoch.div(AISCalendar::SECONDS_PER_DAY))[2]
  end

  # Returns the day of the year in UTC (January 1 is 1).
  # @return [Integer]
  def yday
    days = @epoch.div(AISCalendar::SECONDS_PER_DAY)
    parts = AISCalendar.civil_from_days(days)
    days - AISCalendar.days_from_civil(parts[0], 1, 1) + 1
  end

  # Returns the weekday in UTC. Sunday is 0 and Saturday is 6.
  # @return [Integer]
  def wday
    (@epoch.div(AISCalendar::SECONDS_PER_DAY) + 4) % 7
  end

  # Returns the hour in UTC (0..23).
  # @return [Integer]
  def hour
    seconds_of_day % 86_400 / 3_600
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

  # Adds a duration to this time.
  #
  # @param duration [AISDuration, Numeric] duration in seconds
  # @return [Time]
  def +(duration)
    Time.at(@epoch + duration.to_i)
  end

  # Subtracts another time or a duration.
  #
  # @param other [Time, AISDuration, Numeric]
  # @return [Integer, Time] difference in seconds, or a new time
  def -(other)
    if other.is_a?(Time)
      @epoch - other.to_i
    else
      Time.at(@epoch - other.to_i)
    end
  end

  # Compares timestamps.
  # @param other [Time]
  # @return [Integer, nil] -1, 0, 1, or nil for an unsupported object
  def <=>(other)
    return nil unless other.is_a?(Time)

    @epoch <=> other.to_i
  end

  # Tests timestamp equality.
  # @param other [Object]
  # @return [Boolean]
  def ==(other)
    other.is_a?(Time) && @epoch == other.to_i
  end

  # Returns the Unix timestamp.
  # @return [Integer]
  def to_i
    @epoch
  end

  # Returns this timestamp as an ISO-8601 UTC string.
  # @return [String] formatted as YYYY-MM-DDTHH:MM:SSZ
  def to_s
    parts = AISCalendar.civil_from_days(@epoch.div(AISCalendar::SECONDS_PER_DAY))
    sprintf('%04d-%02d-%02dT%02d:%02d:%02dZ',
            parts[0], parts[1], parts[2], hour, minute, second)
  end

  # Converts this timestamp to a DateTime instance.
  # @return [DateTime]
  def to_datetime
    DateTime.at(@epoch)
  end

  private

  def seconds_of_day
    @epoch - @epoch.div(AISCalendar::SECONDS_PER_DAY) * AISCalendar::SECONDS_PER_DAY
  end
end

# A lightweight duration represented as whole seconds.
#
# Instances are normally created through Numeric extensions such as
# `5.minutes` or `2.days`.
#
# @example
#   15.minutes.ago
#   2.hours.from_now
# @api public
class AISDuration
  # @return [Integer] duration in seconds
  attr_reader :seconds

  # @param seconds [Numeric] duration in seconds
  def initialize(seconds)
    @seconds = seconds.to_i
  end

  # @return [Integer]
  def to_i
    @seconds
  end

  # Returns the time this duration before the current request time.
  # @return [Time]
  def ago
    Time.at(Time.now.to_i - @seconds)
  end

  # Returns the time this duration after the current request time.
  # @return [Time]
  def from_now
    Time.at(Time.now.to_i + @seconds)
  end

  # Adds another duration.
  # @param other [AISDuration, Numeric]
  # @return [AISDuration]
  def +(other)
    AISDuration.new(@seconds + other.to_i)
  end

  # Subtracts another duration.
  # @param other [AISDuration, Numeric]
  # @return [AISDuration]
  def -(other)
    AISDuration.new(@seconds - other.to_i)
  end

  # Multiplies this duration by a numeric factor.
  # @param factor [Numeric]
  # @return [AISDuration]
  def *(factor)
    AISDuration.new(@seconds * factor)
  end

  # Compares durations.
  # @param other [AISDuration]
  # @return [Integer, nil]
  def <=>(other)
    return nil unless other.is_a?(AISDuration)

    @seconds <=> other.to_i
  end
end

# Convenient duration constructors on Numeric.
#
# @example
#   30.seconds
#   5.minutes
#   2.hours
#   3.days
#   1.week
# @see AISDuration
class Numeric
  # @return [AISDuration]
  def seconds
    AISDuration.new(self)
  end

  # @return [AISDuration]
  def second
    seconds
  end

  # @return [AISDuration]
  def minutes
    AISDuration.new(self * 60)
  end

  # @return [AISDuration]
  def minute
    minutes
  end

  # @return [AISDuration]
  def hours
    AISDuration.new(self * 3_600)
  end

  # @return [AISDuration]
  def hour
    hours
  end

  # @return [AISDuration]
  def days
    AISDuration.new(self * 86_400)
  end

  # @return [AISDuration]
  def day
    days
  end

  # @return [AISDuration]
  def weeks
    AISDuration.new(self * 604_800)
  end

  # @return [AISDuration]
  def week
    weeks
  end
end
