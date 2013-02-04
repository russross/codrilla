pragma foreign_keys = on;
pragma encoding = "UTF-8";

create table Administrator (
    Email text primary key not null,
    Name text not null
);

create table Instructor (
    Email text primary key not null,
    Name text not null
);

create table Student (
    Email text primary key not null,
    Name text not null
);

create table Course (
    Tag text primary key not null,
    Name text not null,
    Close timestamp not null
);

create table CourseInstructor (
    Course text not null,
    Instructor text not null,

    primary key (Course, Instructor),
    foreign key (Course) references Course(Tag),
    foreign key (Instructor) references Instructor(Email)
);

create table CourseStudent (
    Course text not null,
    Student text not null,

    primary key (Course, Student),
    foreign key (Course) references Course(Tag),
    foreign key (Student) references Student(Email)
);

create table Problem (
    ID integer primary key autoincrement,
    Name text not null,
    Type text not null,
    Data text not null
);

create table Tag (
    Tag text primary key not null,
    Description text,
    Priority integer not null default 0
);

create table ProblemTag (
    Problem integer not null,
    Tag text not null,

    primary key (Problem, Tag),
    foreign key (Problem) references Problem(ID),
    foreign key (Tag) references Tag(Tag)
);

create table Assignment (
    ID integer primary key autoincrement,
    Course text not null,
    Problem integer not null,
    ForCredit integer not null,
    Open timestamp not null,
    Close timestamp not null,

    foreign key (Course) references Course (Tag),
    foreign key (Problem) references Problem (ID)
);

create table Solution (
    ID integer primary key autoincrement,
    Student text not null,
    Assignment integer not null,

    foreign key (Student) references Student (Email),
    foreign key (Assignment) references Assignment (ID)
);

create table Submission (
    Solution integer not null,
    TimeStamp timestamp not null,
    Submission text not null,
    GradeReport text not null,
    Passed integer,

    primary key (Solution, TimeStamp),
    foreign key (Solution) references Solution (ID)
);
create index submission_timestamp on Submission (TimeStamp);
